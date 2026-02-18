package indexer

import (
	"os"
	"path/filepath"
)

func RenameFile(root, oldRel, newRel string) error {
	oldPath := filepath.Join(root, oldRel)
	newPath := filepath.Join(root, newRel)

	if oldPath == newPath {
		return nil
	}

	// Double check source exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(newPath), os.ModePerm); err != nil {
		return err
	}

	return os.Rename(oldPath, newPath)
}
