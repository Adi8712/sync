package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

type FileMeta struct {
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	ModTime      int64  `json:"mod_time"`
	Hash         string `json:"hash"`
}

type cacheEntry struct {
	Size    int64
	ModTime int64
	Hash    string
}

var (
	cacheMu sync.RWMutex
	cache   = make(map[string]cacheEntry)
)

func ScanFolder(root string) ([]FileMeta, error) {
	var files []FileMeta
	var mu sync.Mutex

	type task struct {
		path    string
		relPath string
		info    os.FileInfo
	}
	tasks := make(chan task, 100)
	var wg sync.WaitGroup

	// Start workers
	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		go func() {
			for t := range tasks {
				hash, _ := getHash(t.path, t.info)
				mu.Lock()
				files = append(files, FileMeta{
					RelativePath: t.relPath,
					Size:         t.info.Size(),
					ModTime:      t.info.ModTime().Unix(),
					Hash:         hash,
				})
				mu.Unlock()
				wg.Done()
			}
		}()
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, _ := filepath.Rel(root, path)
		wg.Add(1)
		tasks <- task{path, relPath, info}
		return nil
	})
	close(tasks)
	wg.Wait()

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})

	return files, err
}

func getHash(path string, info os.FileInfo) (string, error) {
	cacheMu.RLock()
	entry, ok := cache[path]
	cacheMu.RUnlock()

	if ok && entry.Size == info.Size() && entry.ModTime == info.ModTime().Unix() {
		return entry.Hash, nil
	}

	hash, err := hashFile(path)
	if err == nil {
		cacheMu.Lock()
		cache[path] = cacheEntry{
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Hash:    hash,
		}
		cacheMu.Unlock()
	}
	return hash, err
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
