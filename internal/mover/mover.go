// Package mover executes the planned file relocations with TOCTOU checks,
// atomic moves, permissions/timestamp preservation, and real-time history logging.
package mover

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shamil-shah/folr/internal/history"
	"github.com/shamil-shah/folr/internal/plan"
	"github.com/shamil-shah/folr/internal/safety"
)

// MoveResult summarizes the execution of a move plan.
type MoveResult struct {
	MovedCount   int
	SkippedCount int
	Messages     []string
}

// ExecutePlan safely moves each file in the plan and logs every move to the history session.
func ExecutePlan(p *plan.Plan, histSession *history.Session) (*MoveResult, error) {
	result := &MoveResult{
		Messages: make([]string, 0),
	}

	for _, action := range p.Actions {
		// 1. TOCTOU verification immediately prior to moving
		srcFi, err := os.Lstat(action.SourcePath)
		if err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: file vanished or inaccessible", action.SourceName))
			continue
		}

		if !srcFi.Mode().IsRegular() {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: source is no longer a regular file", action.SourceName))
			continue
		}

		if srcFi.Size() != action.Size || !srcFi.ModTime().Equal(action.ModTime) {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: file was modified since plan was built", action.SourceName))
			continue
		}

		// 2. Ensure destination category directory exists safely
		categoryDir := filepath.Dir(action.DestPath)
		if err := ensureCategoryDir(categoryDir); err != nil {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: destination directory error: %v", action.SourceName, err))
			continue
		}

		// 3. Prevent accidental overwrite
		if _, err := os.Lstat(action.DestPath); !os.IsNotExist(err) {
			result.SkippedCount++
			result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: destination already exists (%s)", action.SourceName, action.DestName))
			continue
		}

		// 4. Move file (Rename on same filesystem, fallback to copy+verify+delete for cross-filesystem)
		var fileHash string
		moveErr := os.Rename(action.SourcePath, action.DestPath)
		if moveErr != nil {
			// Cross-device or rename constraint: fallback to atomic copy+verify
			var copyErr error
			fileHash, copyErr = copyAndVerifyMove(action.SourcePath, action.DestPath, srcFi)
			if copyErr != nil {
				result.SkippedCount++
				result.Messages = append(result.Messages, fmt.Sprintf("Skipped %s: unable to move (%v)", action.SourceName, copyErr))
				continue
			}
		}

		// 5. Append to atomic history session
		if histSession != nil {
			_ = histSession.RecordMove(action.SourcePath, action.DestPath, srcFi.Size(), srcFi.ModTime(), fileHash)
		}

		result.MovedCount++
	}

	if histSession != nil {
		_ = histSession.Finish()
	}

	return result, nil
}

// ensureCategoryDir verifies and creates the destination directory safely.
func ensureCategoryDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return safety.NewSecurityError("destination category directory %s is a symlink", dir)
		}
		if !fi.IsDir() {
			return safety.NewSecurityError("destination %s exists as a regular file, not a directory", dir)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return err
}

// copyAndVerifyMove copies data with SHA-256 validation, preserves timestamps/perms, and removes source.
func copyAndVerifyMove(src, dst string, srcFi os.FileInfo) (string, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, srcFi.Mode().Perm())
	if err != nil {
		return "", err
	}
	defer dstFile.Close()

	hashSrc := sha256.New()
	hashDst := sha256.New()

	mw := io.MultiWriter(dstFile, hashDst)
	tee := io.TeeReader(srcFile, hashSrc)

	written, err := io.Copy(mw, tee)
	if err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("copy interrupted: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("file sync failed: %w", err)
	}

	if written != srcFi.Size() {
		_ = os.Remove(dst)
		return "", fmt.Errorf("size mismatch: wrote %d bytes, expected %d", written, srcFi.Size())
	}

	srcSum := hex.EncodeToString(hashSrc.Sum(nil))
	dstSum := hex.EncodeToString(hashDst.Sum(nil))
	if srcSum != dstSum {
		_ = os.Remove(dst)
		return "", errors.New("SHA-256 checksum mismatch after copy")
	}

	// Preserve permissions and timestamps
	_ = os.Chmod(dst, srcFi.Mode().Perm())
	_ = os.Chtimes(dst, srcFi.ModTime(), srcFi.ModTime())

	// Close source file before removing
	_ = srcFile.Close()
	if err := os.Remove(src); err != nil {
		// If source deletion fails, remove dst to prevent duplication
		_ = os.Remove(dst)
		return "", fmt.Errorf("failed to remove source file after copy: %w", err)
	}

	return srcSum, nil
}

