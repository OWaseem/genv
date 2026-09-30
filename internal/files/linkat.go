package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// createSymlinkAt installs source as a symlink at target without ever leaving
// target absent.
//
// The symlink is created first under a unique sibling name and only then
// renamed over the target, so the step that actually fails — os.Symlink on
// Windows without Developer Mode or admin, a read-only directory, a full disk
// — happens before anything is moved. When the live file has to be set aside,
// it is parked (rather than deleted) and restored if the final rename fails.
// The error then names where the original bytes are.
//
// backup controls what happens to an existing non-directory target: true keeps
// a timestamped backup, false replaces it outright. Callers refuse to call this
// for a directory without backup.
func createSymlinkAt(source, target string, backup bool) (string, error) {
	if err := ensureParentDir(target); err != nil {
		return "", err
	}
	info, err := os.Lstat(target)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("link %s: %w", target, err)
	}

	// Stage inside a unique sibling directory so the name cannot collide with
	// anything the user already has, and so the rename stays on one filesystem.
	dir := filepath.Dir(target)
	stageDir, err := os.MkdirTemp(dir, ".genv-link-*")
	if err != nil {
		return "", fmt.Errorf("link %s: %w", target, err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	staged := filepath.Join(stageDir, "link")
	if err := symlink(source, staged); err != nil {
		// Nothing on disk has been touched: the live file is still in place.
		return "", fmt.Errorf("link %s: %w", target, err)
	}

	if !exists {
		if err := os.Rename(staged, target); err != nil {
			return "", fmt.Errorf("link %s: %w", target, err)
		}
		return "", nil
	}

	if backup || info.IsDir() {
		backupPath, err := backupExistingTo(target)
		if err != nil {
			return "", err
		}
		if err := os.Rename(staged, target); err != nil {
			_ = os.Rename(backupPath, target)
			return backupPath, fmt.Errorf("link %s: %w (previous file restored from %s)", target, err, backupPath)
		}
		return backupPath, nil
	}

	// No backup requested, but park the original instead of deleting it: if the
	// final rename fails the bytes are still recoverable, and we drop them only
	// once the replacement link is in place.
	parked := backupPathFor(target)
	if err := os.Rename(target, parked); err != nil {
		return "", fmt.Errorf("link %s: %w", target, err)
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(parked, target)
		return parked, fmt.Errorf("link %s: %w (previous file restored from %s)", target, err, parked)
	}
	_ = os.Remove(parked)
	return "", nil
}
