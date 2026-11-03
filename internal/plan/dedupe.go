// Package plan computes the file organization plan, handles atomic name collisions,
// and enforces destination path boundary checks.
package plan

// DedupeAction will represent duplicate file handling in v0.2.
// In v0.2, duplicates will be identified via size and SHA-256 comparison,
// then moved to a dedicated "Duplicates" folder without deleting originals.
type DedupeAction struct {
	SourcePath   string
	OriginalPath string
	DestPath     string
	Size         int64
	Hash         string
}

// Deduplicator defines the interface for content-based deduplication reserved for v0.2.
type Deduplicator interface {
	FindDuplicates(targetDir string, files []MoveAction) ([]DedupeAction, error)
}

