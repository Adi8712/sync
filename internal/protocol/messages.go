package protocol

import "sync/internal/indexer"

type IndexMessage struct {
	Type  string             `json:"type"`
	Files []indexer.FileMeta `json:"files"`
}

type RequestMessage struct {
	Type   string   `json:"type"`
	Hashes []string `json:"hashes"`
}

type FileHeaderMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}
