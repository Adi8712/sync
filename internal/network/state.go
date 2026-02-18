package network

import (
	"sort"
	"sync"
	"sync/internal/indexer"
)

type NetworkState struct {
	mu    sync.RWMutex
	peers map[string][]indexer.FileMeta
	votes map[string]string
}

func NewNetworkState() *NetworkState {
	return &NetworkState{
		peers: make(map[string][]indexer.FileMeta),
		votes: make(map[string]string),
	}
}

func (s *NetworkState) Update(id string, files []indexer.FileMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[id] = files
}

func (s *NetworkState) SetVote(h, n string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.votes[h] = n
}

func (s *NetworkState) GetGlobal() []indexer.FileMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m := make(map[string]indexer.FileMeta)
	for _, files := range s.peers {
		for _, f := range files {
			m[f.Hash] = f
		}
	}

	var res []indexer.FileMeta
	for _, f := range m {
		res = append(res, f)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].RelativePath < res[j].RelativePath })
	return res
}

func (s *NetworkState) GetWinner(h string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if v, ok := s.votes[h]; ok {
		return v, true
	}

	counts := make(map[string]int)
	for _, files := range s.peers {
		for _, f := range files {
			if f.Hash == h {
				counts[f.RelativePath]++
			}
		}
	}

	max, winner := 0, ""
	tie := false
	for n, c := range counts {
		if c > max {
			max, winner, tie = c, n, false
		} else if c == max {
			tie = true
		}
	}
	return winner, !tie && winner != ""
}
