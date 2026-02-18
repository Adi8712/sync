package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

type FileMeta struct {
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	ModTime      int64  `json:"mod_time"`
	Hash         string `json:"hash"`
}

func ScanFolder(root string) ([]FileMeta, error) {
	var files []FileMeta

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		hash, err := hashFile(path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		files = append(files, FileMeta{
			RelativePath: relPath,
			Size:         info.Size(),
			ModTime:      info.ModTime().Unix(),
			Hash:         hash,
		})

		return nil
	})

	return files, err
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
