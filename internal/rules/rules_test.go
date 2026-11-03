package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	r := DefaultRules()
	if r == nil {
		t.Fatal("expected non-nil default rules")
	}

	tests := []struct {
		ext      string
		expected string
	}{
		{"jpg", "Images"},
		{".JPG", "Images"},
		{"png", "Images"},
		{"pdf", "Documents"},
		{"DOCX", "Documents"},
		{"zip", "Archives"},
		{"mp4", "Videos"},
		{"mp3", "Audio"},
		{"exe", "Installers"},
		{"go", "Code"},
		{"xyz123", "Other"},
		{"", "Other"},
	}

	for _, tt := range tests {
		got := r.GetCategory(tt.ext)
		if got != tt.expected {
			t.Errorf("GetCategory(%q) = %q, expected %q", tt.ext, got, tt.expected)
		}
	}
}

func TestLoadConfigFile_Valid(t *testing.T) {
	tempDir := t.TempDir()
	confPath := filepath.Join(tempDir, "rules.toml")
	content := `
[categories]
Images = ["jpg", "png"]
Books = ["epub", "mobi"]

[settings]
skip_recent_seconds = 5
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := LoadConfigFile(confPath)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}

	if r.GetCategory("epub") != "Books" {
		t.Errorf("expected epub to be Books, got %s", r.GetCategory("epub"))
	}
	if r.SkipRecentSeconds != 5 {
		t.Errorf("expected skip_recent_seconds = 5, got %d", r.SkipRecentSeconds)
	}
}

func TestLoadConfigFile_SizeLimit(t *testing.T) {
	tempDir := t.TempDir()
	confPath := filepath.Join(tempDir, "oversized.toml")

	// Create file slightly over 1MB
	f, err := os.Create(confPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	chunk := []byte(strings.Repeat("# comment line\n", 100))
	for written := 0; written <= MaxConfigFileSize+1024; written += len(chunk) {
		_, _ = f.Write(chunk)
	}

	_, err = LoadConfigFile(confPath)
	if err == nil {
		t.Fatal("expected error for oversized config file, got nil")
	}
}

func TestLoadConfigFile_CategoryLimit(t *testing.T) {
	cats := make(map[string][]string)
	for i := 0; i <= MaxCategoriesLimit+5; i++ {
		cats[fmt.Sprintf("Cat%d", i)] = []string{"ext"}
	}

	_, err := CompileRules(cats, 10)
	if err == nil {
		t.Fatal("expected error exceeding 100 categories, got nil")
	}
}

func TestLoadConfigFile_SymlinkRejected(t *testing.T) {
	tempDir := t.TempDir()
	realPath := filepath.Join(tempDir, "real.toml")
	symlinkPath := filepath.Join(tempDir, "link.toml")

	if err := os.WriteFile(realPath, []byte(`[categories]\nImages = ["jpg"]`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(realPath, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this filesystem/platform")
	}

	_, err := LoadConfigFile(symlinkPath)
	if err == nil {
		t.Fatal("expected symlink config file to be rejected, got nil")
	}
}

