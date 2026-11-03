package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shamil-shah/folr/internal/rules"
	"github.com/shamil-shah/folr/internal/safety"
	"github.com/shamil-shah/folr/internal/scan"
)

// MoveAction represents a planned atomic relocation of a file.
type MoveAction struct {
	SourcePath string
	SourceName string
	DestPath   string
	DestName   string
	Category   string
	Size       int64
	ModTime    time.Time
}

// Plan represents the complete set of planned operations before execution.
type Plan struct {
	TargetDir       string
	Actions         []MoveAction
	CategoryCounts  map[string]int
	CategorySamples map[string][]string
	Skipped         []scan.SkippedFile
	TotalScanned    int
}

// normalizePathKey creates a canonical key for collision tracking across platforms.
func normalizePathKey(p string) string {
	clean := filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(clean)
	}
	return clean
}

// BuildPlan constructs the execution plan from scanned files according to the active rules.
func BuildPlan(scanResult *scan.ScanResult, r *rules.Rules) (*Plan, error) {
	targetDir := scanResult.TargetDir
	plan := &Plan{
		TargetDir:       targetDir,
		Actions:         make([]MoveAction, 0, len(scanResult.Files)),
		CategoryCounts:  make(map[string]int),
		CategorySamples: make(map[string][]string),
		Skipped:         scanResult.Skipped,
		TotalScanned:    len(scanResult.Files) + len(scanResult.Skipped),
	}

	// Track planned destination paths to prevent collisions within the batch
	plannedDests := make(map[string]bool)

	// Validate target directory category paths
	validatedCategories := make(map[string]string) // category -> categoryDir

	for _, file := range scanResult.Files {
		category := r.GetCategory(file.Extension)

		categoryDir, ok := validatedCategories[category]
		if !ok {
			if err := safety.ValidateCategoryName(category); err != nil {
				return nil, err
			}
			categoryDir = filepath.Join(targetDir, category)

			// Path traversal check
			if err := safety.ValidateSafeSubpath(targetDir, categoryDir); err != nil {
				return nil, err
			}

			// Destination check: must not be a symlink or regular file
			if fi, err := os.Lstat(categoryDir); err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					return nil, safety.NewSecurityError("destination category directory %q is a symbolic link", categoryDir)
				}
				if !fi.IsDir() {
					return nil, safety.NewSecurityError("destination category %q exists and is not a directory", categoryDir)
				}
			}
			validatedCategories[category] = categoryDir
		}

		// Resolve destination file name with collision resolution
		destName, destPath, err := resolveDestination(categoryDir, file.Name, plannedDests)
		if err != nil {
			return nil, err
		}

		// Ensure destination file is inside categoryDir
		if err := safety.ValidateSafeSubpath(categoryDir, destPath); err != nil {
			return nil, err
		}

		action := MoveAction{
			SourcePath: file.Path,
			SourceName: file.Name,
			DestPath:   destPath,
			DestName:   destName,
			Category:   category,
			Size:       file.Size,
			ModTime:    file.ModTime,
		}

		plan.Actions = append(plan.Actions, action)
		plan.CategoryCounts[category]++
		if len(plan.CategorySamples[category]) < 2 {
			plan.CategorySamples[category] = append(plan.CategorySamples[category], file.Name)
		}
	}

	return plan, nil
}

// resolveDestination finds an available destination file name, avoiding collisions with existing files
// or files already planned in the same batch (e.g. photo.jpg -> photo (1).jpg -> photo (2).jpg).
func resolveDestination(categoryDir, originalName string, plannedDests map[string]bool) (string, string, error) {
	cleanName := filepath.Base(originalName)

	// Windows disallows trailing dots and spaces in file names
	if runtime.GOOS == "windows" {
		cleanName = strings.TrimRight(cleanName, ". ")
		if cleanName == "" {
			cleanName = "file"
		}
	}

	ext := filepath.Ext(cleanName)
	base := strings.TrimSuffix(cleanName, ext)

	candidateName := cleanName
	candidatePath := filepath.Join(categoryDir, candidateName)
	candidateKey := normalizePathKey(candidatePath)

	counter := 1
	for {
		// Check collision in planned batch
		if !plannedDests[candidateKey] {
			// Check collision on disk
			if _, err := os.Lstat(candidatePath); os.IsNotExist(err) {
				// Slot is available!
				plannedDests[candidateKey] = true
				return candidateName, candidatePath, nil
			}
		}

		// Slot occupied, increment suffix
		candidateName = fmt.Sprintf("%s (%d)%s", base, counter, ext)
		candidatePath = filepath.Join(categoryDir, candidateName)
		candidateKey = normalizePathKey(candidatePath)
		counter++

		if counter > 100000 {
			return "", "", safety.NewSecurityError("unable to resolve collision for file %q: exceeded 100,000 variants", originalName)
		}
	}
}

