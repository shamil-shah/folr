package mover

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shamil-shah/folr/internal/plan"
)

func TestExecutePlan_Normal(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "sample.jpg")
	content := []byte("image binary data")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	srcFi, _ := os.Stat(srcFile)
	destDir := filepath.Join(tempDir, "Images")
	destFile := filepath.Join(destDir, "sample.jpg")

	p := &plan.Plan{
		TargetDir: tempDir,
		Actions: []plan.MoveAction{
			{
				SourcePath: srcFile,
				SourceName: "sample.jpg",
				DestPath:   destFile,
				DestName:   "sample.jpg",
				Category:   "Images",
				Size:       srcFi.Size(),
				ModTime:    srcFi.ModTime(),
			},
		},
	}

	result, err := ExecutePlan(p, nil)
	if err != nil {
		t.Fatalf("unexpected move error: %v", err)
	}

	if result.MovedCount != 1 {
		t.Errorf("expected 1 file moved, got %d", result.MovedCount)
	}

	// Source file should no longer exist at original path
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Error("source file still exists after move")
	}

	// Destination file must exist with correct content
	gotContent, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(gotContent) != string(content) {
		t.Errorf("content mismatch at destination: got %q", string(gotContent))
	}
}

func TestExecutePlan_TOCTOU_Modified(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "doc.pdf")
	if err := os.WriteFile(srcFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	destFile := filepath.Join(tempDir, "Documents", "doc.pdf")

	p := &plan.Plan{
		TargetDir: tempDir,
		Actions: []plan.MoveAction{
			{
				SourcePath: srcFile,
				SourceName: "doc.pdf",
				DestPath:   destFile,
				DestName:   "doc.pdf",
				Category:   "Documents",
				Size:       7,
				ModTime:    time.Now().Add(-10 * time.Minute), // outdated timestamp
			},
		},
	}

	result, err := ExecutePlan(p, nil)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if result.MovedCount != 0 {
		t.Errorf("expected 0 files moved due to TOCTOU mismatch, got %d", result.MovedCount)
	}
	if result.SkippedCount != 1 {
		t.Errorf("expected 1 file skipped, got %d", result.SkippedCount)
	}

	// Source file must remain intact!
	if _, err := os.Stat(srcFile); err != nil {
		t.Error("source file was deleted despite TOCTOU failure")
	}
}

