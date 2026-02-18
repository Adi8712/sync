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

var (
	cacheMu sync.RWMutex
	cache   = make(map[string]FileMeta)
)

func ScanFolder(root string) ([]FileMeta, error) {
	var files []FileMeta
	var mu sync.Mutex
	var wg sync.WaitGroup

	tasks := make(chan string, 100)
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for path := range tasks {
				if f, err := getMeta(root, path); err == nil {
					mu.Lock()
					files = append(files, f)
					mu.Unlock()
				}
				wg.Done()
			}
		}()
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			wg.Add(1)
			tasks <- path
		}
		return err
	})
	close(tasks)
	wg.Wait()

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, err
}

func getMeta(root, path string) (FileMeta, error) {
	inf, err := os.Stat(path)
	if err != nil {
		return FileMeta{}, err
	}

	cacheMu.RLock()
	c, ok := cache[path]
	cacheMu.RUnlock()

	if ok && c.Size == inf.Size() && c.ModTime == inf.ModTime().Unix() {
		return c, nil
	}

	h, err := hashFile(path)
	if err != nil {
		return FileMeta{}, err
	}

	rel, _ := filepath.Rel(root, path)
	f := FileMeta{rel, inf.Size(), inf.ModTime().Unix(), h}

	cacheMu.Lock()
	cache[path] = f
	cacheMu.Unlock()
	return f, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
