//go:build windows

package scan

import (
	"os"
	"syscall"
)

const (
	fileAttributeHidden = 0x02
	fileAttributeSystem = 0x04
)

// isHiddenOrSystemOS checks if the file has Windows HIDDEN or SYSTEM attributes.
func isHiddenOrSystemOS(fi os.FileInfo) bool {
	if sys, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		if sys.FileAttributes&(fileAttributeHidden|fileAttributeSystem) != 0 {
			return true
		}
	}
	return false
}

