package network

import "sync/internal/indexer"

type IndexMessage struct {
	Type     string             `json:"type"`
	DeviceID string             `json:"device_id"`
	Files    []indexer.FileMeta `json:"files"`
}

type FileHeaderMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type ConsensusVoteMessage struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	Name string `json:"name"`
}

type FileRequestMessage struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	Path string `json:"path"`
}

type DoneMessage struct {
	Type string `json:"type"`
}
