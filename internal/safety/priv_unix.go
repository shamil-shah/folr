//go:build !windows

package safety

import "os"

// IsPrivilegedUser returns true if the current process is running with root privileges on Unix.
func IsPrivilegedUser() (bool, error) {
	return os.Geteuid() == 0, nil
}

