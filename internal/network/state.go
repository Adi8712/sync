package network

import (
	"sort"
	"sync"
	"sync/internal/indexer"
)

type FileVoter struct {
	Names map[string]int // Name -> Count
}

type NetworkState struct {
	mu           sync.RWMutex
	Peers        map[string][]indexer.FileMeta // DeviceID -> List of files
	GlobalCounts map[string]*FileVoter         // Hash -> Voter
	ManualVotes  map[string]string             // Hash -> Manual Winner
}

func NewNetworkState() *NetworkState {
	return &NetworkState{
		Peers:        make(map[string][]indexer.FileMeta),
		GlobalCounts: make(map[string]*FileVoter),
		ManualVotes:  make(map[string]string),
	}
}

func (s *NetworkState) SetManualConsensus(hash, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ManualVotes[hash] = name
}

func (s *NetworkState) UpdatePeer(deviceID string, files []indexer.FileMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Peers[deviceID] = files
	s.recalculateConsensus()
}

func (s *NetworkState) recalculateConsensus() {
	newCounts := make(map[string]*FileVoter)

	for _, files := range s.Peers {
		for _, f := range files {
			voter, ok := newCounts[f.Hash]
			if !ok {
				voter = &FileVoter{Names: make(map[string]int)}
				newCounts[f.Hash] = voter
			}
			voter.Names[f.RelativePath]++
		}
	}
	s.GlobalCounts = newCounts
}

func (s *NetworkState) GetConsensusName(hash string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if name, ok := s.ManualVotes[hash]; ok {
		return name, true
	}

	voter, ok := s.GlobalCounts[hash]
	if !ok {
		return "", false
	}

	maxCount := -1
	var winner string
	isTie := false

	for name, count := range voter.Names {
		if count > maxCount {
			maxCount = count
			winner = name
			isTie = false
		} else if count == maxCount {
			isTie = true
		}
	}

	return winner, !isTie
}

func (s *NetworkState) GetGlobalFiles() []indexer.FileMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []indexer.FileMeta
	hashesSeen := make(map[string]bool)

	for hash, voter := range s.GlobalCounts {
		if hashesSeen[hash] {
			continue
		}

		// Find best name
		var bestName string
		max := -1
		for name, count := range voter.Names {
			if count > max {
				max = count
				bestName = name
			}
		}

		// Just need any file meta for this hash from any peer to get size/modtime
		for _, peerFiles := range s.Peers {
			for _, f := range peerFiles {
				if f.Hash == hash {
					result = append(result, indexer.FileMeta{
						RelativePath: bestName,
						Size:         f.Size,
						ModTime:      f.ModTime,
						Hash:         f.Hash,
					})
					hashesSeen[hash] = true
					goto nextHash
				}
			}
		}
	nextHash:
	}

	// Sort results alphabetically by name
	sort.Slice(result, func(i, j int) bool {
		return result[i].RelativePath < result[j].RelativePath
	})

	return result
}
