// Package safety provides security checks, path sanitization, forbidden directory
// validations, privilege checks, and directory lock management for folr.
package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ExitSuccess          = 0
	ExitGeneralError     = 1
	ExitMisuse           = 2
	ExitSecurityRejected = 3

	MaxScannedFiles = 100000
)

// SecurityError represents a violation of security constraints.
type SecurityError struct {
	Reason string
}

func (e *SecurityError) Error() string {
	return fmt.Sprintf("security restriction violated: %s", e.Reason)
}

// NewSecurityError creates a new SecurityError.
func NewSecurityError(format string, args ...any) error {
	return &SecurityError{Reason: fmt.Sprintf(format, args...)}
}

// categoryNameRegex allows only alphanumeric characters, spaces, hyphens, and underscores.
var categoryNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)

// windowsReservedNames lists reserved device names on Windows.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// IsReservedWindowsName checks if a name matches any Windows reserved file or device name.
func IsReservedWindowsName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	// Strip extension if present (e.g., CON.txt is also reserved on Windows)
	base := upper
	if idx := strings.Index(base, "."); idx != -1 {
		base = base[:idx]
	}
	return windowsReservedNames[base] || windowsReservedNames[upper]
}

// ValidateCategoryName ensures category names are strictly safe:
// no slashes, no traversal, no control characters, within length limits, and not Windows reserved.
func ValidateCategoryName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return NewSecurityError("category name cannot be empty")
	}
	if len(trimmed) > 100 {
		return NewSecurityError("category name exceeds maximum length of 100 characters: %q", trimmed)
	}
	if !categoryNameRegex.MatchString(trimmed) {
		return NewSecurityError("category name %q contains invalid characters (only letters, digits, spaces, '-' and '_' allowed)", trimmed)
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, "/\\<>:\"|?*") {
		return NewSecurityError("category name %q contains forbidden path or wildcard characters", trimmed)
	}
	if IsReservedWindowsName(trimmed) {
		return NewSecurityError("category name %q is a Windows reserved device name", trimmed)
	}
	return nil
}

// forbiddenUnixRoots lists critical system directories in Linux/Unix.
var forbiddenUnixRoots = []string{
	"/", "/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/boot",
	"/dev", "/proc", "/sys", "/var", "/root", "/home", "/Users",
}

// forbiddenWindowsNames lists critical system directories in Windows.
var forbiddenWindowsNames = []string{
	"WINDOWS", "PROGRAM FILES", "PROGRAM FILES (X86)", "PROGRAMDATA",
}

// ValidateTargetDirectory verifies that the target directory exists, is a directory,
// is not a symlink, and is not a forbidden critical system directory or user home root.
func ValidateTargetDirectory(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", NewSecurityError("target directory path cannot be empty")
	}

	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", NewSecurityError("failed to resolve absolute path for %q: %v", target, err)
	}

	// Verify target itself is not a symlink
	absFi, err := os.Lstat(absPath)
	if err != nil {
		return "", NewSecurityError("cannot access or evaluate target path %q: %v", target, err)
	}
	if absFi.Mode()&os.ModeSymlink != 0 {
		return "", NewSecurityError("target directory %q is a symlink; folr refuses to operate on symlinks", target)
	}
	if !absFi.IsDir() {
		return "", NewSecurityError("target %q is not a directory", target)
	}

	// Follow symlinks (if any parent directories are symlinks) to get canonical real path for validation
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", NewSecurityError("cannot access or evaluate target path %q: %v", target, err)
	}

	cleanAbs := filepath.Clean(absPath)
	cleanReal := filepath.Clean(realPath)

	for _, p := range []string{cleanAbs, cleanReal} {
		// Check filesystem root
		if p == "/" || p == filepath.VolumeName(p)+string(filepath.Separator) {
			return "", NewSecurityError("refusing to operate on filesystem root directory (%s)", p)
		}

		// Check user home directory (subdirectories like ~/Downloads are permitted, but home directory itself is forbidden)
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if realHome, err := filepath.EvalSymlinks(home); err == nil {
				if p == filepath.Clean(realHome) || p == filepath.Clean(home) {
					return "", NewSecurityError("refusing to operate directly on user home directory (%s). Run on a subdirectory like ~/Downloads instead.", p)
				}
			}
		}

		// Check Unix forbidden directories and their direct subdirectories for critical system areas
		cleanSlash := filepath.ToSlash(p)
		for _, forbidden := range forbiddenUnixRoots {
			if cleanSlash == forbidden {
				return "", NewSecurityError("refusing to operate on forbidden system directory: %s", forbidden)
			}
			// Also forbid subdirectories for critical system roots (e.g. /etc/..., /boot/..., /dev/..., /proc/..., /sys/...)
			if forbidden == "/etc" || forbidden == "/boot" || forbidden == "/dev" || forbidden == "/proc" || forbidden == "/sys" || forbidden == "/root" {
				if strings.HasPrefix(cleanSlash, forbidden+"/") {
					return "", NewSecurityError("refusing to operate inside critical system directory: %s", p)
				}
			}
		}

		// Check Windows forbidden system directories
		vol := filepath.VolumeName(p)
		pathWithoutVol := p
		if vol != "" {
			pathWithoutVol = p[len(vol):]
		}
		parts := strings.Split(strings.Trim(filepath.ToSlash(pathWithoutVol), "/"), "/")
		if len(parts) > 0 {
			topFolder := strings.ToUpper(parts[0])
			for _, forbidden := range forbiddenWindowsNames {
				if topFolder == forbidden {
					return "", NewSecurityError("refusing to operate on Windows system directory: %s", p)
				}
			}
		}
	}

	return cleanAbs, nil
}

// ValidateSafeSubpath verifies that targetPath resides strictly inside baseDir,
// preventing path traversal attacks (e.g., using '..' or symlinks).
func ValidateSafeSubpath(baseDir, targetPath string) error {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err == nil {
		relSlash := filepath.ToSlash(rel)
		if relSlash != ".." && !strings.HasPrefix(relSlash, "../") && !filepath.IsAbs(rel) {
			return nil
		}
	}

	// If direct string comparison failed (e.g. Windows 8.3 short name alias vs long name),
	// attempt comparison via canonical evaluated paths.
	realBase, errBase := filepath.EvalSymlinks(baseDir)
	if errBase == nil {
		realTarget, errTarget := filepath.EvalSymlinks(targetPath)
		if errTarget != nil {
			// If targetPath does not exist yet (e.g. undo restore destination), evaluate parent directory
			dir := filepath.Dir(targetPath)
			if realDir, errDir := filepath.EvalSymlinks(dir); errDir == nil {
				realTarget = filepath.Join(realDir, filepath.Base(targetPath))
				errTarget = nil
			}
		}

		if errTarget == nil {
			relReal, errRel := filepath.Rel(realBase, realTarget)
			if errRel == nil {
				relRealSlash := filepath.ToSlash(relReal)
				if relRealSlash != ".." && !strings.HasPrefix(relRealSlash, "../") && !filepath.IsAbs(relReal) {
					return nil
				}
			}
		}
	}

	return NewSecurityError("path traversal detected: %q escapes base directory %q", targetPath, baseDir)
}

// CheckNotSymlink verifies that the item at path is not a symlink.
func CheckNotSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return NewSecurityError("symlink detected at %q; symlink operations are prohibited", path)
	}
	return nil
}

