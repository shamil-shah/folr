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

func TestValidateSafeSubpath_CanonicalEquivalence(t *testing.T) {
	tempDir := t.TempDir()
	realDir := filepath.Join(tempDir, "real_base")
	_ = os.Mkdir(realDir, 0755)
	aliasDir := filepath.Join(tempDir, "alias_base")
	if err := os.Symlink(realDir, aliasDir); err == nil {
		targetFile := filepath.Join(aliasDir, "sub", "file.txt")
		_ = os.MkdirAll(filepath.Dir(targetFile), 0755)
		_ = os.WriteFile(targetFile, []byte("ok"), 0644)

		// baseDir is realDir, targetPath uses aliasDir:
		if err := ValidateSafeSubpath(realDir, targetFile); err != nil {
			t.Errorf("expected canonical resolution to allow subpath, got error: %v", err)
		}

		// baseDir is aliasDir, targetPath uses realDir:
		if err := ValidateSafeSubpath(aliasDir, filepath.Join(realDir, "sub", "file.txt")); err != nil {
			t.Errorf("expected canonical resolution to allow subpath, got error: %v", err)
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

	// Symlink to directory should be rejected
	subDir := filepath.Join(tempDir, "real")
	_ = os.Mkdir(subDir, 0755)
	symLink := filepath.Join(tempDir, "sym")
	if err := os.Symlink(subDir, symLink); err == nil {
		if _, err := ValidateTargetDirectory(symLink); err == nil {
			t.Error("expected symlink directory to be rejected, got nil")
		}
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
