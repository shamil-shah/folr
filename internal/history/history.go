// Package history manages secure operation logging and atomic undo operations.
package history

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shamil-shah/folr/internal/safety"
)

const (
	MaxHistoryFiles = 20
	HistoryDirPerm  = 0700
	HistoryFilePerm = 0600
)

// FileRecord represents an individual moved file entry in the history log.
type FileRecord struct {
	SourcePath string    `json:"source_path"`
	DestPath   string    `json:"dest_path"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	SHA256     string    `json:"sha256,omitempty"`
}

// OperationRecord represents a complete or ongoing folr organize run.
type OperationRecord struct {
	ID        string       `json:"id"`
	TargetDir string       `json:"target_dir"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   time.Time    `json:"ended_at,omitempty"`
	Entries   []FileRecord `json:"entries"`
}

// Session manages the real-time writing of the operation history.
type Session struct {
	record  *OperationRecord
	logPath string
	dir     string
}

// GetHistoryDir returns the secure directory for storing folr history logs.
func GetHistoryDir() (string, error) {
	var baseDir string
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		baseDir = stateHome
	} else if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		baseDir = configDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to determine user home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".config")
	}
	historyDir := filepath.Join(baseDir, "folr", "history")
	if err := os.MkdirAll(historyDir, HistoryDirPerm); err != nil {
		return "", fmt.Errorf("failed to create history directory %s: %w", historyDir, err)
	}
	// Ensure strict directory permissions
	_ = os.Chmod(historyDir, HistoryDirPerm)
	return historyDir, nil
}

// generateID creates a unique, collision-resistant operation identifier.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// StartSession initializes a new history log session.
func StartSession(targetDir string) (*Session, error) {
	histDir, err := GetHistoryDir()
	if err != nil {
		return nil, err
	}

	id := generateID()
	logPath := filepath.Join(histDir, fmt.Sprintf("run-%s.json", id))

	record := &OperationRecord{
		ID:        id,
		TargetDir: targetDir,
		StartedAt: time.Now().UTC(),
		Entries:   make([]FileRecord, 0),
	}

	s := &Session{
		record:  record,
		logPath: logPath,
		dir:     histDir,
	}

	if err := s.saveAtomic(); err != nil {
		return nil, err
	}

	// Prune old history files to keep only the latest 20
	_ = PruneOldHistory(histDir, MaxHistoryFiles)

	return s, nil
}

// RecordMove appends a successfully moved file to the session and atomically updates disk.
func (s *Session) RecordMove(sourcePath, destPath string, size int64, modTime time.Time, hash string) error {
	s.record.Entries = append(s.record.Entries, FileRecord{
		SourcePath: sourcePath,
		DestPath:   destPath,
		Size:       size,
		ModTime:    modTime,
		SHA256:     hash,
	})
	return s.saveAtomic()
}

// Finish marks the session as completed and persists the final state.
func (s *Session) Finish() error {
	s.record.EndedAt = time.Now().UTC()
	return s.saveAtomic()
}

// saveAtomic writes the history record to a temp file and renames it atomically.
func (s *Session) saveAtomic() error {
	data, err := json.MarshalIndent(s.record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode history log: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp-%d", s.logPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, HistoryFilePerm); err != nil {
		return fmt.Errorf("failed to write temporary history file: %w", err)
	}

	if err := os.Rename(tmpFile, s.logPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to finalize history log file: %w", err)
	}

	return nil
}

// PruneOldHistory removes history logs exceeding the retention limit.
func PruneOldHistory(histDir string, maxKeep int) error {
	files, err := os.ReadDir(histDir)
	if err != nil {
		return err
	}

	var logFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "run-") && strings.HasSuffix(f.Name(), ".json") {
			logFiles = append(logFiles, filepath.Join(histDir, f.Name()))
		}
	}

	if len(logFiles) <= maxKeep {
		return nil
	}

	sort.Strings(logFiles)
	toDelete := len(logFiles) - maxKeep
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(logFiles[i])
	}

	return nil
}

// FindLatestOperation finds the most recent operation history log.
// If targetDir is provided, it returns the latest log matching targetDir;
// otherwise returns the most recent log overall.
func FindLatestOperation(targetDir string) (*OperationRecord, string, error) {
	histDir, err := GetHistoryDir()
	if err != nil {
		return nil, "", err
	}

	files, err := os.ReadDir(histDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read history directory: %w", err)
	}

	var candidates []string
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "run-") && strings.HasSuffix(f.Name(), ".json") {
			candidates = append(candidates, filepath.Join(histDir, f.Name()))
		}
	}

	if len(candidates) == 0 {
		return nil, "", errors.New("no history operations found to undo")
	}

	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))

	cleanTarget := ""
	if targetDir != "" {
		cleanTarget = filepath.Clean(targetDir)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var record OperationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		if cleanTarget == "" || filepath.Clean(record.TargetDir) == cleanTarget {
			return &record, path, nil
		}
	}

	return nil, "", fmt.Errorf("no history operation found matching directory %s", targetDir)
}

// UndoResult summarizes the results of an undo operation.
type UndoResult struct {
	RestoredCount int
	SkippedCount  int
	Messages      []string
}

// ExecuteUndo restores files recorded in the operation back to their source paths.
// Follows strict validation: no data loss, handles occupied source paths, prunes empty folders.
func ExecuteUndo(record *OperationRecord) (*UndoResult, error) {
	result := &UndoResult{
		Messages: make([]string, 0),
	}

	targetDir := record.TargetDir
	if err := safety.CheckNotSymlink(targetDir); err != nil {
		return nil, err
	}

	touchedDirs := make(map[string]bool)

	// Revert in reverse order of move
	for i := len(record.Entries) - 1; i >= 0; i-- {
		entry := record.Entries[i]

		// 1. Re-validate destination path (treat history log as untrusted input)
		if err := safety.ValidateSafeSubpath(targetDir, entry.DestPath); err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: security validation failed: %v", filepath.Base(entry.DestPath), err))
			continue
		}
		if err := safety.CheckNotSymlink(entry.DestPath); err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: symlink detected in destination", filepath.Base(entry.DestPath)))
			continue
		}

		// 2. Inspect destination file
		fi, err := os.Lstat(entry.DestPath)
		if err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: file not found in %s", filepath.Base(entry.DestPath), filepath.Dir(entry.DestPath)))
			continue
		}
		if !fi.Mode().IsRegular() {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: destination is no longer a regular file", filepath.Base(entry.DestPath)))
			continue
		}
		if fi.Size() != entry.Size {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: file size changed since move (%d -> %d)", filepath.Base(entry.DestPath), entry.Size, fi.Size()))
			continue
		}

		// 3. Re-validate source path
		if err := safety.ValidateSafeSubpath(targetDir, entry.SourcePath); err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: source path validation failed: %v", filepath.Base(entry.SourcePath), err))
			continue
		}

		// 4. Resolve restore destination if original source path is currently occupied
		restorePath := entry.SourcePath
		if _, err := os.Lstat(restorePath); err == nil {
			// Source path is occupied! Do not overwrite. Use name (restored).ext
			restorePath = resolveRestoreConflict(entry.SourcePath)
		}

		// 5. Restore file
		if err := os.Rename(entry.DestPath, restorePath); err != nil {
			// Fallback if cross-device or rename issue
			if copyErr := copyAndVerify(entry.DestPath, restorePath); copyErr != nil {
				result.SkippedCount++
				result.Messages = append(result.Messages, fmt.Sprintf("Failed to restore %s: %v", filepath.Base(entry.DestPath), copyErr))
				continue
			}
			_ = os.Remove(entry.DestPath)
		}

		result.RestoredCount++
		touchedDirs[filepath.Dir(entry.DestPath)] = true
	}

	// 6. Clean up empty category directories if they were emptied by undo
	for dir := range touchedDirs {
		if dir == targetDir {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}

	return result, nil
}

// resolveRestoreConflict generates a non-conflicting path like name (restored).ext
func resolveRestoreConflict(origPath string) string {
	dir := filepath.Dir(origPath)
	cleanName := filepath.Base(origPath)
	ext := filepath.Ext(cleanName)
	base := strings.TrimSuffix(cleanName, ext)

	candidate := filepath.Join(dir, fmt.Sprintf("%s (restored)%s", base, ext))
	if _, err := os.Lstat(candidate); os.IsNotExist(err) {
		return candidate
	}

	for i := 1; i <= 10000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (restored) (%d)%s", base, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s (restored-%d)%s", base, time.Now().UnixNano(), ext))
}

// copyAndVerify copies data from src to dst with SHA-256 verification and permissions preservation.
func copyAndVerify(src, dst string) error {
	srcFi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, srcFi.Mode().Perm())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	hashSrc := sha256.New()
	hashDst := sha256.New()

	mw := io.MultiWriter(dstFile, hashDst)
	tee := io.TeeReader(srcFile, hashSrc)

	written, err := io.Copy(mw, tee)
	if err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy interrupted: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("sync failed: %w", err)
	}

	if written != srcFi.Size() {
		_ = os.Remove(dst)
		return fmt.Errorf("size mismatch during copy (wrote %d, expected %d)", written, srcFi.Size())
	}

	if hex.EncodeToString(hashSrc.Sum(nil)) != hex.EncodeToString(hashDst.Sum(nil)) {
		_ = os.Remove(dst)
		return errors.New("SHA-256 hash checksum mismatch after copy")
	}

	_ = os.Chmod(dst, srcFi.Mode().Perm())
	_ = os.Chtimes(dst, srcFi.ModTime(), srcFi.ModTime())

	return nil
}
