//go:build !windows

package safety

import (
	"errors"
	"syscall"
)

// isProcessAlive returns true if the process with the given PID is running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// EPERM means process exists but we do not have permission to signal it
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

