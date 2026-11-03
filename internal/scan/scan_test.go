package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shamil-shah/folr/internal/rules"
)

func TestScanDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Regular candidate file (older than 10 seconds)
	oldFile := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(oldFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-20 * time.Second)
	_ = os.Chtimes(oldFile, oldTime, oldTime)

	// 2. Subdirectory (must be ignored completely)
	subDir := filepath.Join(tempDir, "SubFolder")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	// File inside subfolder should never be touched
	if err := os.WriteFile(filepath.Join(subDir, "nested.jpg"), []byte("nested"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Incomplete downloads
	partFile := filepath.Join(tempDir, "archive.zip.crdownload")
	_ = os.WriteFile(partFile, []byte("part"), 0644)
	_ = os.Chtimes(partFile, oldTime, oldTime)

	tmpFile := filepath.Join(tempDir, "data.tmp")
	_ = os.WriteFile(tmpFile, []byte("tmp"), 0644)
	_ = os.Chtimes(tmpFile, oldTime, oldTime)

	// 4. Hidden file
	hiddenFile := filepath.Join(tempDir, ".hidden_doc.pdf")
	_ = os.WriteFile(hiddenFile, []byte("secret"), 0644)
	_ = os.Chtimes(hiddenFile, oldTime, oldTime)

	// 5. Folr lock and config
	lockFile := filepath.Join(tempDir, ".folr.lock")
	_ = os.WriteFile(lockFile, []byte("lock"), 0600)
	_ = os.Chtimes(lockFile, oldTime, oldTime)

	// 6. Recent file (modified just now, < 10 seconds)
	recentFile := filepath.Join(tempDir, "active_write.txt")
	_ = os.WriteFile(recentFile, []byte("writing..."), 0644)

	// 7. Symlink (if supported)
	symlinkTarget := filepath.Join(tempDir, "target.txt")
	_ = os.WriteFile(symlinkTarget, []byte("target"), 0644)
	_ = os.Chtimes(symlinkTarget, oldTime, oldTime)
	symlinkPath := filepath.Join(tempDir, "symlink_file.txt")
	_ = os.Symlink(symlinkTarget, symlinkPath)

	// 8. Own executable binary file (must be skipped)
	execPath, _ := os.Executable()
	execName := filepath.Base(execPath)
	ownBinary := filepath.Join(tempDir, execName)
	_ = os.WriteFile(ownBinary, []byte("fake binary"), 0755)
	_ = os.Chtimes(ownBinary, oldTime, oldTime)

	r := rules.DefaultRules()
	res, err := ScanDirectory(tempDir, r)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}

	// Validate candidate files: photo.jpg and target.txt should be candidate files
	candidates := make(map[string]bool)
	for _, f := range res.Files {
		candidates[f.Name] = true
	}

	if candidates[execName] {
		t.Errorf("running binary %s should never be a candidate file to move", execName)
	}

	if !candidates["photo.jpg"] {
		t.Error("expected photo.jpg to be in candidate files")
	}
	if !candidates["target.txt"] {
		t.Error("expected target.txt to be in candidate files")
	}
	if candidates["nested.jpg"] {
		t.Error("nested.jpg from subdirectory should never be scanned")
	}
	if candidates["archive.zip.crdownload"] {
		t.Error("incomplete download .crdownload should not be a candidate file")
	}
	if candidates[".hidden_doc.pdf"] {
		t.Error("hidden file should not be a candidate file")
	}
	if candidates[".folr.lock"] {
		t.Error(".folr.lock should not be a candidate file")
	}
	if candidates["active_write.txt"] {
		t.Error("active recently modified file should not be a candidate file")
	}

	// Validate skipped files
	skippedReasons := make(map[string]string)
	for _, s := range res.Skipped {
		skippedReasons[s.Name] = s.Reason
	}

	if _, ok := skippedReasons["archive.zip.crdownload"]; !ok {
		t.Error("expected archive.zip.crdownload to be recorded as skipped")
	}
	if _, ok := skippedReasons[".hidden_doc.pdf"]; !ok {
		t.Error("expected .hidden_doc.pdf to be recorded as skipped")
	}
	if _, ok := skippedReasons["active_write.txt"]; !ok {
		t.Error("expected active_write.txt to be recorded as skipped due to recent modification")
	}
	if _, ok := skippedReasons["symlink_file.txt"]; !ok {
		t.Error("expected symlink_file.txt to be recorded as skipped symlink")
	}
}

