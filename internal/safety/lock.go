package safety

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const LockFileName = ".folr.lock"
const staleLockDuration = 15 * time.Minute

// LockPayload stores execution metadata written into .folr.lock
type LockPayload struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
	Hostname  string    `json:"hostname,omitempty"`
}

// DirLock represents an active lock on a target directory.
type DirLock struct {
	path string
}

// Path returns the path of the lock file.
func (l *DirLock) Path() string {
	return l.path
}

// Release removes the lock file.
func (l *DirLock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file %s: %w", l.path, err)
	}
	l.path = ""
	return nil
}

// AcquireLock attempts to acquire an exclusive lock on the target directory.
// If a stale lock is detected (process dead or older than 15 minutes), it cleans up and retries once.
func AcquireLock(targetDir string) (*DirLock, error) {
	lockPath := filepath.Join(targetDir, LockFileName)

	tryAcquire := func() (*DirLock, error) {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		hostname, _ := os.Hostname()
		payload := LockPayload{
			PID:       os.Getpid(),
			CreatedAt: time.Now().UTC(),
			Hostname:  hostname,
		}
		if err := json.NewEncoder(f).Encode(&payload); err != nil {
			_ = os.Remove(lockPath)
			return nil, fmt.Errorf("failed to write lock payload: %w", err)
		}
		return &DirLock{path: lockPath}, nil
	}

	lock, err := tryAcquire()
	if err == nil {
		return lock, nil
	}

	if !os.IsExist(err) {
		return nil, fmt.Errorf("unable to create lock file: %w", err)
	}

	// Lock file exists. Inspect it for stale lock condition.
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		// If lock file exists but cannot be read, check mod time
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > staleLockDuration {
			_ = os.Remove(lockPath)
			return tryAcquire()
		}
		return nil, NewSecurityError("lock file exists at %s and cannot be read", lockPath)
	}

	var payload LockPayload
	isStale := false
	if json.Unmarshal(data, &payload) != nil {
		// Invalid JSON content in lock file
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > staleLockDuration {
			isStale = true
		}
	} else {
		// Valid payload: check if process is alive or lock is too old
		if time.Since(payload.CreatedAt) > staleLockDuration || !isProcessAlive(payload.PID) {
			isStale = true
		}
	}

	if isStale {
		_ = os.Remove(lockPath)
		return tryAcquire()
	}

	return nil, NewSecurityError("another folr process (PID: %d, started at: %s) is currently operating on %s",
		payload.PID, payload.CreatedAt.Format(time.RFC3339), targetDir)
}

