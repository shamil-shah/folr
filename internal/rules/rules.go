// Package rules provides category definitions, extension mappings, TOML configuration loading,
// and strict input validation according to folr security specifications.
package rules

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/shamil-shah/folr/internal/safety"
)

const (
	MaxConfigFileSize    = 1024 * 1024 // 1 MB limit
	MaxCategoriesLimit   = 100
	DefaultSkipRecentSec = 10
	OtherCategory        = "Other"
)

// DefaultCategories returns the built-in category and extension mapping.
var DefaultCategories = map[string][]string{
	"Images":     {"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "heic", "tiff"},
	"Documents":  {"pdf", "doc", "docx", "odt", "txt", "rtf", "md", "xls", "xlsx", "ods", "ppt", "pptx", "csv"},
	"Archives":   {"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "tgz"},
	"Videos":     {"mp4", "mkv", "avi", "mov", "webm", "flv"},
	"Audio":      {"mp3", "wav", "flac", "ogg", "m4a", "aac"},
	"Installers": {"exe", "msi", "deb", "rpm", "dmg", "appimage", "apk"},
	"Code":       {"py", "go", "js", "ts", "c", "cpp", "java", "rs", "sh", "json", "yaml", "yml", "html", "css"},
}

// ConfigFile represents the TOML file structure.
type ConfigFile struct {
	Categories map[string][]string `toml:"categories"`
	Settings   Settings            `toml:"settings"`
}

// Settings stores configurable parameters.
type Settings struct {
	SkipRecentSeconds int `toml:"skip_recent_seconds"`
}

// Rules holds the compiled and validated extension-to-category lookup.
type Rules struct {
	extToCat          map[string]string
	Categories        []string
	SkipRecentSeconds int
}

// GetCategory returns the category for a given file extension (e.g. "jpg" or ".JPG").
// If no specific category matches, it returns "Other".
func (r *Rules) GetCategory(ext string) string {
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if cat, found := r.extToCat[cleanExt]; found {
		return cat
	}
	return OtherCategory
}

// CompileRules compiles category-to-extension mappings into an optimized lookup structure.
func CompileRules(categories map[string][]string, skipRecentSeconds int) (*Rules, error) {
	if len(categories) > MaxCategoriesLimit {
		return nil, safety.NewSecurityError("configuration exceeds category limit of %d (found %d)", MaxCategoriesLimit, len(categories))
	}

	extMap := make(map[string]string)
	var catList []string
	seenCat := make(map[string]bool)

	for catName, exts := range categories {
		if err := safety.ValidateCategoryName(catName); err != nil {
			return nil, err
		}

		if !seenCat[catName] {
			seenCat[catName] = true
			catList = append(catList, catName)
		}

		for _, ext := range exts {
			cleanExt := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(ext, ".")))
			if cleanExt == "" {
				continue
			}
			// If already claimed by another category, first defined takes precedence
			if _, exists := extMap[cleanExt]; !exists {
				extMap[cleanExt] = catName
			}
		}
	}

	if skipRecentSeconds <= 0 {
		skipRecentSeconds = DefaultSkipRecentSec
	}

	return &Rules{
		extToCat:          extMap,
		Categories:        catList,
		SkipRecentSeconds: skipRecentSeconds,
	}, nil
}

// DefaultRules returns the compiled default rules with standard categories.
func DefaultRules() *Rules {
	r, _ := CompileRules(DefaultCategories, DefaultSkipRecentSec)
	return r
}

// LoadRules discovers and loads configuration in strict order:
// 1. Explicit --config flag
// 2. .folr.toml in target directory
// 3. User config directory ($XDG_CONFIG_HOME/folr/rules.toml or os.UserConfigDir()/folr/rules.toml)
// 4. Built-in defaults
func LoadRules(configFlag, targetDir string) (*Rules, error) {
	var configPath string

	if configFlag != "" {
		configPath = configFlag
	} else {
		// Check target directory for .folr.toml
		localConfig := filepath.Join(targetDir, ".folr.toml")
		if fi, err := os.Lstat(localConfig); err == nil && !fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 {
			configPath = localConfig
		} else {
			// Check user configuration directory
			if userConfigDir, err := os.UserConfigDir(); err == nil && userConfigDir != "" {
				globalConfig := filepath.Join(userConfigDir, "folr", "rules.toml")
				if fi, err := os.Lstat(globalConfig); err == nil && !fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 {
					configPath = globalConfig
				}
			}
		}
	}

	if configPath == "" {
		return DefaultRules(), nil
	}

	return LoadConfigFile(configPath)
}

// LoadConfigFile reads and parses a TOML configuration file with security boundaries.
func LoadConfigFile(path string) (*Rules, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, safety.NewSecurityError("config file %q is a symbolic link", path)
	}

	if fi.IsDir() {
		return nil, fmt.Errorf("config path %q is a directory, expected file", path)
	}

	if fi.Size() > MaxConfigFileSize {
		return nil, safety.NewSecurityError("config file %q exceeds 1MB limit (%d bytes)", path, fi.Size())
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}
	defer f.Close()

	// Read with size limiter
	limitedReader := io.LimitReader(f, MaxConfigFileSize+1)
	buf, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	if int64(len(buf)) > MaxConfigFileSize {
		return nil, safety.NewSecurityError("config file exceeds maximum allowed size of 1MB")
	}

	var parsed ConfigFile
	if _, err := toml.NewDecoder(bytes.NewReader(buf)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("invalid TOML syntax in %q: %w", path, err)
	}

	// If no categories provided in config, fallback to default categories
	categories := parsed.Categories
	if len(categories) == 0 {
		categories = DefaultCategories
	}

	skipSeconds := parsed.Settings.SkipRecentSeconds
	if skipSeconds <= 0 {
		skipSeconds = DefaultSkipRecentSec
	}

	return CompileRules(categories, skipSeconds)
}

