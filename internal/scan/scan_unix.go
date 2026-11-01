//go:build !windows

package scan

import "os"

// isHiddenOrSystemOS checks for OS-specific hidden or system attributes.
// On Unix, hidden files are handled by dot prefix in scan.go.
func isHiddenOrSystemOS(fi os.FileInfo) bool {
	return false
}

