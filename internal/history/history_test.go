package history

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUndo_Normal(t *testing.T) {
	tempDir := t.TempDir()

	// Simulate moved file in Images/photo.jpg
	imagesDir := filepath.Join(tempDir, "Images")
	if err := os.Mkdir(imagesDir, 0755); err != nil {
		t.Fatal(err)
	}

	destFile := filepath.Join(imagesDir, "photo.jpg")
	content := []byte("hello photo")
	if err := os.WriteFile(destFile, content, 0644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(destFile)

	origSource := filepath.Join(tempDir, "photo.jpg")

	record := &OperationRecord{
		ID:        "test-1",
		TargetDir: tempDir,
		StartedAt: time.Now(),
		Entries: []FileRecord{
			{
				SourcePath: origSource,
				DestPath:   destFile,
				Size:       fi.Size(),
				ModTime:    fi.ModTime(),
			},
		},
	}

	result, err := ExecuteUndo(record)
	if err != nil {
		t.Fatalf("unexpected undo error: %v", err)
	}

	if result.RestoredCount != 1 {
		t.Errorf("expected 1 restored file, got %d", result.RestoredCount)
	}

	// Verify restored file exists at origSource
	if _, err := os.Stat(origSource); err != nil {
		t.Errorf("restored file does not exist at %s: %v", origSource, err)
	}

	// Verify empty category directory Images was cleaned up
	if _, err := os.Stat(imagesDir); !os.IsNotExist(err) {
		t.Error("expected empty category directory to be removed after undo")
	}
}

func TestUndo_SourceOccupied(t *testing.T) {
	tempDir := t.TempDir()

	imagesDir := filepath.Join(tempDir, "Images")
	_ = os.Mkdir(imagesDir, 0755)

	destFile := filepath.Join(imagesDir, "report.pdf")
	_ = os.WriteFile(destFile, []byte("report content"), 0644)
	fi, _ := os.Stat(destFile)

	origSource := filepath.Join(tempDir, "report.pdf")
	// Place an existing file at origSource to cause an occupation collision
	_ = os.WriteFile(origSource, []byte("new conflicting content"), 0644)

	record := &OperationRecord{
		ID:        "test-2",
		TargetDir: tempDir,
		StartedAt: time.Now(),
		Entries: []FileRecord{
			{
				SourcePath: origSource,
				DestPath:   destFile,
				Size:       fi.Size(),
				ModTime:    fi.ModTime(),
			},
		},
	}

	result, err := ExecuteUndo(record)
	if err != nil {
		t.Fatalf("unexpected undo error: %v", err)
	}

	if result.RestoredCount != 1 {
		t.Errorf("expected 1 restored file, got %d", result.RestoredCount)
	}

	// Original occupied file must be untouched!
	origContent, _ := os.ReadFile(origSource)
	if string(origContent) != "new conflicting content" {
		t.Error("original occupied file was overwritten!")
	}

	// Restored file should exist as report (restored).pdf
	restoredPath := filepath.Join(tempDir, "report (restored).pdf")
	restoredContent, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("restored file not found at %s: %v", restoredPath, err)
	}
	if string(restoredContent) != "report content" {
		t.Errorf("restored file content mismatch: got %q", string(restoredContent))
	}
}

func TestUndo_ModifiedOrMissingDestFile(t *testing.T) {
	tempDir := t.TempDir()

	imagesDir := filepath.Join(tempDir, "Images")
	_ = os.Mkdir(imagesDir, 0755)

	destFile := filepath.Join(imagesDir, "file.txt")
	// File was modified by user after folr ran
	_ = os.WriteFile(destFile, []byte("modified longer content"), 0644)

	record := &OperationRecord{
		ID:        "test-3",
		TargetDir: tempDir,
		StartedAt: time.Now(),
		Entries: []FileRecord{
			{
				SourcePath: filepath.Join(tempDir, "file.txt"),
				DestPath:   destFile,
				Size:       4, // original size was 4 bytes
				ModTime:    time.Now().Add(-1 * time.Hour),
			},
			{
				SourcePath: filepath.Join(tempDir, "missing.txt"),
				DestPath:   filepath.Join(imagesDir, "missing.txt"),
				Size:       10,
				ModTime:    time.Now(),
			},
		},
	}

	result, err := ExecuteUndo(record)
	if err != nil {
		t.Fatalf("unexpected undo error: %v", err)
	}

	if result.RestoredCount != 0 {
		t.Errorf("expected 0 restored files, got %d", result.RestoredCount)
	}
	if result.SkippedCount != 2 {
		t.Errorf("expected 2 skipped files, got %d", result.SkippedCount)
	}
}

func TestUndo_PathTraversalRejected(t *testing.T) {
	tempDir := t.TempDir()

	// Malicious log entry trying to escape targetDir
	record := &OperationRecord{
		ID:        "malicious-log",
		TargetDir: tempDir,
		StartedAt: time.Now(),
		Entries: []FileRecord{
			{
				SourcePath: filepath.Join(tempDir, "..", "..", "etc", "malicious.txt"),
				DestPath:   filepath.Join(tempDir, "Images", "payload.txt"),
				Size:       10,
				ModTime:    time.Now(),
			},
		},
	}

	result, err := ExecuteUndo(record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RestoredCount != 0 {
		t.Errorf("malicious path traversal was executed! Restored count: %d", result.RestoredCount)
	}
}

func TestPruneOldHistory(t *testing.T) {
	tempDir := t.TempDir()

	// Create 25 mock log files
	for i := 1; i <= 25; i++ {
		name := fmt.Sprintf("run-%03d.json", i)
		_ = os.WriteFile(filepath.Join(tempDir, name), []byte("{}"), 0600)
	}

	if err := PruneOldHistory(tempDir, 20); err != nil {
		t.Fatalf("failed to prune history: %v", err)
	}

	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 20 {
		t.Errorf("expected 20 history files remaining, got %d", len(entries))
	}
}

