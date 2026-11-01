package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCategoryName(t *testing.T) {
	valid := []string{
		"Images", "Documents", "My Files", "Archive-2024", "code_repo", "mp3 123",
	}
	for _, name := range valid {
		if err := ValidateCategoryName(name); err != nil {
			t.Errorf("expected valid category name %q, got error: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"Images/Sub",
		"Images\\Sub",
		"../escape",
		"foo*bar",
		"file<1>",
		"CON", "con", "prn", "AUX", "nul", "com1", "lpt9",
		"CON.txt",
		string(make([]byte, 101)), // exceeds 100 chars
	}
	for _, name := range invalid {
		if err := ValidateCategoryName(name); err == nil {
			t.Errorf("expected error for invalid category name %q, got nil", name)
		}
	}
}

func TestValidateSafeSubpath(t *testing.T) {
	baseDir := filepath.Join("home", "user", "folder")

	safe := []string{
		filepath.Join(baseDir, "Images"),
		filepath.Join(baseDir, "Images", "photo.jpg"),
		filepath.Join(baseDir, "file.txt"),
	}
	for _, p := range safe {
		if err := ValidateSafeSubpath(baseDir, p); err != nil {
			t.Errorf("expected safe subpath %q, got error: %v", p, err)
		}
	}

	unsafe := []string{
		filepath.Join(baseDir, "..", "other"),
		filepath.Join(baseDir, "..", "..", "etc", "passwd"),
		filepath.Clean(string(filepath.Separator) + filepath.Join("etc", "passwd")),
	}
	for _, p := range unsafe {
		if err := ValidateSafeSubpath(baseDir, p); err == nil {
			t.Errorf("expected path traversal detection for %q, got nil", p)
		}
	}
}

func TestValidateTargetDirectory_Forbidden(t *testing.T) {
	// Root directory should be rejected
	if _, err := ValidateTargetDirectory("/"); err == nil {
		t.Error("expected root '/' to be rejected, got nil")
	}

	// /etc should be rejected
	if _, err := ValidateTargetDirectory("/etc"); err == nil {
		t.Error("expected '/etc' to be rejected, got nil")
	}

	// User home directory itself should be rejected
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if _, err := ValidateTargetDirectory(home); err == nil {
			t.Errorf("expected home directory %s to be rejected, got nil", home)
		}
	}
}

func TestValidateTargetDirectory_Valid(t *testing.T) {
	tempDir := t.TempDir()
	validated, err := ValidateTargetDirectory(tempDir)
	if err != nil {
		t.Fatalf("expected valid tempDir, got error: %v", err)
	}
	if validated == "" {
		t.Fatal("expected non-empty validated directory")
	}
}

func TestAcquireLock_ExclusiveAndRelease(t *testing.T) {
	tempDir := t.TempDir()

	lock1, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}
	defer lock1.Release()

	// Second attempt should fail while first lock is held
	lock2, err := AcquireLock(tempDir)
	if err == nil {
		lock2.Release()
		t.Fatal("expected second lock acquisition to fail, but succeeded")
	}

	// Release first lock
	if err := lock1.Release(); err != nil {
		t.Fatalf("failed to release lock1: %v", err)
	}

	// Now third attempt should succeed
	lock3, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("expected lock acquisition to succeed after release, got: %v", err)
	}
	_ = lock3.Release()
}

func TestAcquireLock_StaleLock(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, LockFileName)

	// Write a fake stale lock with non-existent PID and older time
	staleContent := `{"pid": 99999999, "created_at": "2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(lockPath, []byte(staleContent), 0600); err != nil {
		t.Fatalf("failed to write fake lock file: %v", err)
	}

	// AcquireLock should detect stale lock and acquire successfully
	lock, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("expected stale lock to be cleaned and acquired, got error: %v", err)
	}
	_ = lock.Release()
}
