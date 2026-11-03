// Package scan scans the target directory for direct regular files, enforcing resource limits
// and identifying files that must be skipped according to folr safety rules.
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shamil-shah/folr/internal/rules"
	"github.com/shamil-shah/folr/internal/safety"
)

// IncompleteDownloadExts lists file extensions representing in-progress downloads.
var IncompleteDownloadExts = map[string]bool{
	".crdownload": true,
	".part":       true,
	".partial":    true,
	".tmp":        true,
	".download":   true,
	".opdownload": true,
}

// ScannedFile represents a valid candidate regular file to be organized.
type ScannedFile struct {
	Name      string
	Path      string
	Size      int64
	ModTime   time.Time
	Extension string
}

// SkippedFile records an ignored file along with the safety reason.
type SkippedFile struct {
	Name   string
	Path   string
	Reason string
}

// ScanResult contains all candidate files and skipped entries.
type ScanResult struct {
	TargetDir string
	Files     []ScannedFile
	Skipped   []SkippedFile
}

// ScanDirectory inspects direct files in targetDir, enforcing all skip and security criteria.
func ScanDirectory(targetDir string, r *rules.Rules) (*ScanResult, error) {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	if len(entries) > safety.MaxScannedFiles {
		return nil, safety.NewSecurityError("directory contains %d items, exceeding the maximum limit of %d", len(entries), safety.MaxScannedFiles)
	}

	result := &ScanResult{
		TargetDir: targetDir,
		Files:     make([]ScannedFile, 0),
		Skipped:   make([]SkippedFile, 0),
	}

	now := time.Now()
	skipRecentSec := r.SkipRecentSeconds
	if skipRecentSec <= 0 {
		skipRecentSec = rules.DefaultSkipRecentSec
	}
	recentThreshold := time.Duration(skipRecentSec) * time.Second

	// Identify current running executable to never self-organize the folr binary
	execPath, _ := os.Executable()
	realExecPath, _ := filepath.EvalSymlinks(execPath)
	execName := filepath.Base(execPath)

	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(targetDir, name)

		// 1. Skip subdirectories: strictly only direct files are inspected
		if entry.IsDir() {
			continue
		}

		// 2. Inspect with Lstat to never follow symlinks
		fi, err := os.Lstat(fullPath)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: fmt.Sprintf("cannot inspect: %v", err),
			})
			continue
		}

		// 3. Skip symbolic links
		if fi.Mode()&os.ModeSymlink != 0 {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "symlink",
			})
			continue
		}

		// 4. Skip non-regular files (devices, pipes, sockets)
		if !fi.Mode().IsRegular() {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "special device/pipe/socket",
			})
			continue
		}

		// 5. Skip folr's own configuration and lock files
		if name == safety.LockFileName || name == ".folr.toml" || name == ".folr" || strings.HasPrefix(name, ".folr.") {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "folr internal file",
			})
			continue
		}

		// Skip folr's own executable binary if running directly in targetDir
		if (execName != "" && strings.EqualFold(name, execName)) || (realExecPath != "" && fullPath == realExecPath) {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "folr executable binary",
			})
			continue
		}

		// 6. Skip hidden files (starting with '.')
		if strings.HasPrefix(name, ".") {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "hidden file",
			})
			continue
		}

		// 7. Platform-specific attributes (Windows Hidden / System)
		if isHiddenOrSystemOS(fi) {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "system or hidden file attribute",
			})
			continue
		}

		// 8. Skip incomplete download files
		ext := strings.ToLower(filepath.Ext(name))
		if IncompleteDownloadExts[ext] {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: "incomplete download",
			})
			continue
		}

		// 9. Skip files modified within recent threshold (may still be actively written)
		if now.Sub(fi.ModTime()) < recentThreshold {
			result.Skipped = append(result.Skipped, SkippedFile{
				Name:   name,
				Path:   fullPath,
				Reason: fmt.Sprintf("modified within last %d seconds", skipRecentSec),
			})
			continue
		}

		result.Files = append(result.Files, ScannedFile{
			Name:      name,
			Path:      fullPath,
			Size:      fi.Size(),
			ModTime:   fi.ModTime(),
			Extension: ext,
		})
	}

	return result, nil
}

