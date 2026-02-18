package sync

import "sync/internal/indexer"

type DiffResult struct {
	MissingInA []indexer.FileMeta
	MissingInB []indexer.FileMeta
	Conflicts  []Conflict
}

type Conflict struct {
	Path string
	A    indexer.FileMeta
	B    indexer.FileMeta
}

func Compare(aFiles, bFiles []indexer.FileMeta) DiffResult {
	result := DiffResult{}

	aMap := make(map[string]indexer.FileMeta)
	bMap := make(map[string]indexer.FileMeta)

	for _, f := range aFiles {
		aMap[f.RelativePath] = f
	}

	for _, f := range bFiles {
		bMap[f.RelativePath] = f
	}

	for path, bFile := range bMap {
		aFile, exists := aMap[path]
		if !exists {
			result.MissingInA = append(result.MissingInA, bFile)
			continue
		}

		if aFile.Hash != bFile.Hash {
			result.Conflicts = append(result.Conflicts, Conflict{
				Path: path,
				A:    aFile,
				B:    bFile,
			})
		}
	}

	for path, aFile := range aMap {
		if _, exists := bMap[path]; !exists {
			result.MissingInB = append(result.MissingInB, aFile)
		}
	}

	return result
}
