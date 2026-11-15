// Package repository is the storage layer: concrete, in-memory
// implementations of the usecase package's store interfaces (RoomStore,
// StageStore, StreamerStore, CurrentRoomProvider). The usecase package
// defines those ports; this package is the adapter main.go wires into them.
package repository

import (
	"context"
	"sort"
	"sync"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

// Store is the server's entire in-memory state: the one open room
// (context.md: "one room per server"), its registered streamers and its
// stage. A single struct guarded by one mutex backs the narrow RoomStore,
// StageStore and StreamerStore interfaces each usecase depends on.
type Store struct {
	mu sync.RWMutex

	room *roomRecord

	streamers map[string]streamerRecord // by streamer_id

	stageVersion    int64
	stageTemplate   *string
	stagePlacements []dto.Placement
}

type roomRecord struct {
	id        string
	creatorIP string
}

type streamerRecord struct {
	streamerID  string
	peerID      string
	displayName string
}

func NewStore() *Store {
	return &Store{streamers: make(map[string]streamerRecord)}
}

// --- RoomStore ---

// IsExist reports whether roomId is the currently open room.
func (s *Store) IsExist(ctx context.Context, roomId string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.room != nil && s.room.id == roomId, nil
}

// OwnerIp reports the IP that opened roomId, or domain.ErrRoomNotFound if
// roomId isn't the currently open room.
func (s *Store) OwnerIp(ctx context.Context, roomId string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.room == nil || s.room.id != roomId {
		return "", domain.ErrRoomNotFound
	}
	return s.room.creatorIP, nil
}

// Open opens roomId for creatorIp. It fails with *domain.RoomExistsError if
// a room is already open.
func (s *Store) Open(ctx context.Context, roomId, creatorIp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.room != nil {
		return &domain.RoomExistsError{RoomID: s.room.id}
	}
	s.room = &roomRecord{id: roomId, creatorIP: creatorIp}
	return nil
}

// Close drops roomId along with its streamers and stage. It fails with
// domain.ErrRoomNotFound if roomId isn't the currently open room.
func (s *Store) Close(ctx context.Context, roomId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.room == nil || s.room.id != roomId {
		return domain.ErrRoomNotFound
	}
	s.room = nil
	s.streamers = make(map[string]streamerRecord)
	s.stageVersion = 0
	s.stageTemplate = nil
	s.stagePlacements = nil
	return nil
}

// Current reports the currently open room, if any.
func (s *Store) Current(ctx context.Context) (roomId string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.room == nil {
		return "", false
	}
	return s.room.id, true
}

// --- StageStore ---

// RoomExists reports whether roomId is the currently open room.
func (s *Store) RoomExists(ctx context.Context, roomId string) (bool, error) {
	return s.IsExist(ctx, roomId)
}

// GetRoomLayout returns the current stage.
func (s *Store) GetRoomLayout(ctx context.Context, roomId string) (domain.RoomLayout, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return domain.RoomLayout{Version: s.stageVersion, Template: s.stageTemplate, Placements: s.stagePlacements}, nil
}

// SetRoomLayout replaces the template and placements and bumps the version.
func (s *Store) SetRoomLayout(ctx context.Context, roomId string, layout domain.RoomLayout) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stageTemplate = layout.Template
	s.stagePlacements = layout.Placements
	s.stageVersion++
	return nil
}

// --- StreamerStore ---

// Exists reports whether streamerId is registered.
func (s *Store) Exists(ctx context.Context, streamerId string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.streamers[streamerId]
	return ok, nil
}

// Upsert registers or updates a streamer and bumps the stage version
// (contracts.md: "version goes up by 1 on every change to the layout or
// the streamers").
func (s *Store) Upsert(ctx context.Context, streamerId, peerId, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamers[streamerId] = streamerRecord{streamerID: streamerId, peerID: peerId, displayName: displayName}
	s.stageVersion++
	return nil
}

// List returns every registered streamer, sorted by streamer_id for a
// stable order. Live is always false; callers that need it cross-reference
// the SFU's current publisher list themselves.
func (s *Store) List(ctx context.Context) ([]dto.Streamer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dto.Streamer, 0, len(s.streamers))
	for _, rec := range s.streamers {
		out = append(out, dto.Streamer{StreamerID: rec.streamerID, PeerID: rec.peerID, DisplayName: rec.displayName})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StreamerID < out[j].StreamerID })
	return out, nil
}
