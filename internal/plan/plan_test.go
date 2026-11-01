package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shamil-shah/folr/internal/rules"
	"github.com/shamil-shah/folr/internal/scan"
)

func TestBuildPlan_CollisionHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-create destination directory and an existing file in it
	imagesDir := filepath.Join(tempDir, "Images")
	if err := os.Mkdir(imagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingFile := filepath.Join(imagesDir, "photo.jpg")
	if err := os.WriteFile(existingFile, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Prepare scanned file with identical name in targetDir
	file1 := scan.ScannedFile{
		Name:      "photo.jpg",
		Path:      filepath.Join(tempDir, "photo.jpg"),
		Size:      100,
		ModTime:   time.Now().Add(-1 * time.Minute),
		Extension: ".jpg",
	}

	scanRes := &scan.ScanResult{
		TargetDir: tempDir,
		Files:     []scan.ScannedFile{file1},
	}

	r := rules.DefaultRules()
	p, err := BuildPlan(scanRes, r)
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}

	if len(p.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(p.Actions))
	}

	// Because photo.jpg exists, it should be resolved to photo (1).jpg
	action := p.Actions[0]
	expectedName := "photo (1).jpg"
	if action.DestName != expectedName {
		t.Errorf("expected destination name %q, got %q", expectedName, action.DestName)
	}
}

func TestBuildPlan_BatchCollisions(t *testing.T) {
	tempDir := t.TempDir()

	// Two scanned files that have the same destination name (e.g. from different casing on Windows or same name)
	file1 := scan.ScannedFile{
		Name:      "document.pdf",
		Path:      filepath.Join(tempDir, "document.pdf"),
		Size:      100,
		ModTime:   time.Now().Add(-1 * time.Minute),
		Extension: ".pdf",
	}
	file2 := scan.ScannedFile{
		Name:      "document.pdf", // duplicate name in batch simulation
		Path:      filepath.Join(tempDir, "document.pdf"),
		Size:      100,
		ModTime:   time.Now().Add(-1 * time.Minute),
		Extension: ".pdf",
	}

	scanRes := &scan.ScanResult{
		TargetDir: tempDir,
		Files:     []scan.ScannedFile{file1, file2},
	}

	r := rules.DefaultRules()
	p, err := BuildPlan(scanRes, r)
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}

	if len(p.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(p.Actions))
	}

	if p.Actions[0].DestName == p.Actions[1].DestName {
		t.Errorf("expected distinct destination names, got both %s", p.Actions[0].DestName)
	}
}

func TestBuildPlan_SymlinkCategoryRejected(t *testing.T) {
	tempDir := t.TempDir()
	otherDir := t.TempDir()

	// Create symlink Images -> otherDir
	symlinkPath := filepath.Join(tempDir, "Images")
	if err := os.Symlink(otherDir, symlinkPath); err != nil {
		t.Skip("symlinks not supported")
	}

	file1 := scan.ScannedFile{
		Name:      "sample.jpg",
		Path:      filepath.Join(tempDir, "sample.jpg"),
		Size:      50,
		ModTime:   time.Now().Add(-1 * time.Minute),
		Extension: ".jpg",
	}

	scanRes := &scan.ScanResult{
		TargetDir: tempDir,
		Files:     []scan.ScannedFile{file1},
	}

	r := rules.DefaultRules()
	_, err := BuildPlan(scanRes, r)
	if err == nil {
		t.Fatal("expected error when category directory is a symlink, got nil")
	}
}

