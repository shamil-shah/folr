//go:build windows

package safety

import (
	"syscall"
)

const (
	processQueryLimitedInformation = 0x1000
	processQueryInformation        = 0x0400
	stillActive                    = 259
)

// isProcessAlive returns true if the process with the given PID is running on Windows.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryInformation|processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var code uint32
	err = syscall.GetExitCodeProcess(h, &code)
	if err != nil {
		return false
	}
	return code == stillActive
}

