package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shamil-shah/folr/internal/safety"
)

// testArgs ensures --allow-root is supplied if running in an elevated environment (e.g. Windows CI Administrator)
func testArgs(args ...string) []string {
	isPriv, _ := safety.IsPrivilegedUser()
	if isPriv {
		return append([]string{"--allow-root"}, args...)
	}
	return args
}

func TestCLI_OrganizeAndUndo(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files older than 10 seconds
	file1 := filepath.Join(tempDir, "photo.jpg")
	file2 := filepath.Join(tempDir, "notes.txt")
	_ = os.WriteFile(file1, []byte("image bytes"), 0644)
	_ = os.WriteFile(file2, []byte("text notes"), 0644)

	past := time.Now().Add(-20 * time.Second)
	_ = os.Chtimes(file1, past, past)
	_ = os.Chtimes(file2, past, past)

	// 1. Run folr with -y on tempDir
	exitCode := run(testArgs("-y", tempDir))
	if exitCode != safety.ExitSuccess {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Verify files moved to Images and Documents
	movedImage := filepath.Join(tempDir, "Images", "photo.jpg")
	movedDoc := filepath.Join(tempDir, "Documents", "notes.txt")

	if _, err := os.Stat(movedImage); err != nil {
		t.Errorf("expected photo.jpg in Images/: %v", err)
	}
	if _, err := os.Stat(movedDoc); err != nil {
		t.Errorf("expected notes.txt in Documents/: %v", err)
	}
	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Error("expected original photo.jpg to have moved from targetDir root")
	}

	// 2. Run folr --undo on tempDir
	undoCode := run(testArgs("--undo", tempDir))
	if undoCode != safety.ExitSuccess {
		t.Fatalf("expected undo exit code 0, got %d", undoCode)
	}

	// Verify files restored back
	if _, err := os.Stat(file1); err != nil {
		t.Errorf("expected photo.jpg to be restored to root: %v", err)
	}
	if _, err := os.Stat(file2); err != nil {
		t.Errorf("expected notes.txt to be restored to root: %v", err)
	}

	// Verify category directories cleaned up
	if _, err := os.Stat(filepath.Join(tempDir, "Images")); !os.IsNotExist(err) {
		t.Error("expected empty Images folder to be deleted after undo")
	}
}

func TestCLI_DryRun_ChangesNothing(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "pic.png")
	_ = os.WriteFile(file1, []byte("png"), 0644)
	past := time.Now().Add(-20 * time.Second)
	_ = os.Chtimes(file1, past, past)

	exitCode := run(testArgs("--dry-run", tempDir))
	if exitCode != safety.ExitSuccess {
		t.Fatalf("expected exit code 0 for dry-run, got %d", exitCode)
	}

	// File must still be at root
	if _, err := os.Stat(file1); err != nil {
		t.Errorf("dry run moved or altered file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "Images")); !os.IsNotExist(err) {
		t.Error("dry run created directory Images")
	}
}

func TestCLI_DangerousDirectory_ExitCode3(t *testing.T) {
	// Attempting to run on "/" must be rejected with exit code 3
	exitCode := run([]string{"-y", "/"})
	if exitCode != safety.ExitSecurityRejected {
		t.Errorf("expected exit code %d (ExitSecurityRejected), got %d", safety.ExitSecurityRejected, exitCode)
	}

	// Attempting to run on "/etc" must be rejected with exit code 3
	exitCodeEtc := run([]string{"-y", "/etc"})
	if exitCodeEtc != safety.ExitSecurityRejected {
		t.Errorf("expected exit code %d (ExitSecurityRejected) for /etc, got %d", safety.ExitSecurityRejected, exitCodeEtc)
	}
}

func TestCLI_FolderWithSpaces(t *testing.T) {
	tempDir := t.TempDir()
	folderWithSpace := filepath.Join(tempDir, "My Downloads")
	if err := os.Mkdir(folderWithSpace, 0755); err != nil {
		t.Fatal(err)
	}

	file1 := filepath.Join(folderWithSpace, "song.mp3")
	_ = os.WriteFile(file1, []byte("mp3 audio"), 0644)
	past := time.Now().Add(-20 * time.Second)
	_ = os.Chtimes(file1, past, past)

	exitCode := run(testArgs("-y", folderWithSpace))
	if exitCode != safety.ExitSuccess {
		t.Fatalf("expected exit code 0 for directory with spaces, got %d", exitCode)
	}

	movedAudio := filepath.Join(folderWithSpace, "Audio", "song.mp3")
	if _, err := os.Stat(movedAudio); err != nil {
		t.Errorf("expected song.mp3 in Audio/: %v", err)
	}
}

func TestCLI_VersionAndHelp(t *testing.T) {
	if code := run([]string{"--version"}); code != safety.ExitSuccess {
		t.Errorf("expected code 0 for --version, got %d", code)
	}
	if code := run([]string{"--help"}); code != safety.ExitSuccess {
		t.Errorf("expected code 0 for --help, got %d", code)
	}
	if code := run([]string{"--invalid-flag"}); code != safety.ExitMisuse {
		t.Errorf("expected code 2 for invalid flag, got %d", code)
	}
}
