package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path so that readers (and a crash or power
// loss mid-write) only ever see the old or the new content, never a
// truncated file. The data goes to a temporary file in the same directory,
// which is synced and then renamed over path. The file always ends up with
// perm, even if an existing file had looser permissions.
//
// If path is a symlink (e.g. managed by a dotfiles tool), the symlink's target
// is written instead of replacing the symlink with a regular file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}

	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmp.Name()

	// Remove the temporary file unless it was renamed into place
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	renamed = true

	// Make the rename itself durable (best effort - not supported everywhere)
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}

	return nil
}
