package repository

import (
	"context"
	"sort"
	"sync"

	"capstonewebserver/internal/dto"
)

// BlacklistStore is the in-memory store of which IPs are currently blocked
// from the room, and which peer_id each block was recorded against. It's
// the single source of truth shared by usecase.BlacklistUseCase (which
// adds/removes entries) and middleware.BlackListMiddleware (which only
// reads IsBlocked).
type BlacklistStore struct {
	mu    sync.RWMutex
	peers map[string]string // ip -> peer_id
}

func NewBlacklistStore() *BlacklistStore {
	return &BlacklistStore{peers: make(map[string]string)}
}

// IsBlocked reports whether ip is currently blacklisted.
func (s *BlacklistStore) IsBlocked(ctx context.Context, ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, blocked := s.peers[ip]
	return blocked
}

// Add blocks ip for peerId and returns the peer_id now on record for it.
// If ip was already blocked, the existing record is left alone and its
// peer_id is returned instead.
func (s *BlacklistStore) Add(ctx context.Context, ip, peerId string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, blocked := s.peers[ip]; blocked {
		return existing, nil
	}
	s.peers[ip] = peerId
	return peerId, nil
}

// Remove unblocks ip. removed is false if ip wasn't blocked.
func (s *BlacklistStore) Remove(ctx context.Context, ip string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, blocked := s.peers[ip]; !blocked {
		return false, nil
	}
	delete(s.peers, ip)
	return true, nil
}

// List returns every blocked {ip, peer_id} pair, sorted by ip for a stable
// order.
func (s *BlacklistStore) List(ctx context.Context) ([]dto.BlacklistEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dto.BlacklistEntry, 0, len(s.peers))
	for ip, peerId := range s.peers {
		out = append(out, dto.BlacklistEntry{IP: ip, PeerID: peerId})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out, nil
}

// Clear drops every entry.
func (s *BlacklistStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = make(map[string]string)
	return nil
}
