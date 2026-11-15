// Package usecase holds the application's business logic. Every type this
// package depends on outside of domain/dto values is expressed as a port
// (an interface declared here); internal/repository provides the storage
// adapters, internal/httpclient the SFU adapter, and main.go wires them
// together. usecase never imports repository or httpclient.
package usecase

import (
	"context"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=usecase.go -destination=mock_ports_test.go -package=usecase

// RequestClient is Go's view of the Rust SFU's internal REST API
// (contracts.md section 2).
type RequestClient interface {
	// GetPublishers lists the peer_ids currently publishing in roomId.
	GetPublishers(ctx context.Context, roomId string) ([]dto.Publisher, error)
	// GetPeerIP resolves peerId to the IP it connected from. Returns
	// domain.ErrPeerNotFound if peerId isn't connected to roomId right now.
	GetPeerIP(ctx context.Context, roomId, peerId string) (string, error)
	// DisconnectPeer tells the SFU to close peerId's connections and drop it
	// from roomId. Returns domain.ErrPeerNotFound if peerId isn't connected
	// to roomId right now.
	DisconnectPeer(ctx context.Context, roomId, peerId string) error
	// CloseRoom tells the SFU to disconnect and drop roomId. Also succeeds
	// if the SFU has no such room.
	CloseRoom(ctx context.Context, roomId string) error
}

// RoomStore is RoomUseCase's storage port.
type RoomStore interface {
	IsExist(ctx context.Context, roomId string) (bool, error)
	OwnerIp(ctx context.Context, roomId string) (string, error)
	// Open opens roomId for creatorIp. *domain.RoomExistsError if a room is
	// already open.
	Open(ctx context.Context, roomId, creatorIp string) error
	// Close drops roomId along with its streamers and stage.
	// domain.ErrRoomNotFound if no room/different room.
	Close(ctx context.Context, roomId string) error
}

// StageStore is LayoutUseCase's storage port.
type StageStore interface {
	RoomExists(ctx context.Context, roomId string) (bool, error)
	GetRoomLayout(ctx context.Context, roomId string) (domain.RoomLayout, error)
	SetRoomLayout(ctx context.Context, roomId string, layout domain.RoomLayout) error
}

// StreamerStore is the streamer registry, keyed by streamer_id. It backs
// StreamerUseCase and LayoutUseCase (which needs the registered streamers
// to build the stage response).
type StreamerStore interface {
	RoomExists(ctx context.Context, roomId string) (bool, error)
	Exists(ctx context.Context, streamerId string) (bool, error)
	Upsert(ctx context.Context, streamerId, peerId, displayName string) error
	List(ctx context.Context) ([]dto.Streamer, error)
}

// CurrentRoomProvider reports the server's one open room, if any.
type CurrentRoomProvider interface {
	Current(ctx context.Context) (roomId string, ok bool)
}

// BlackListStore is BlacklistUseCase's storage port: the blocked-IP set
// (shared with middleware.IPBlacklistChecker, which only reads IsBlocked)
// plus the peer_id recorded per IP for ListBlacklist.
type BlackListStore interface {
	// IsBlocked reports whether ip is currently blacklisted.
	IsBlocked(ctx context.Context, ip string) bool
	// Add blocks ip for peerId and returns the peer_id now on record for
	// it. If ip was already blocked, the existing record is left alone and
	// its peer_id is returned instead (contracts.md: "blocking an IP
	// that's already blocked returns the existing entry").
	Add(ctx context.Context, ip, peerId string) (recordedPeerId string, err error)
	// Remove unblocks ip. removed is false if ip wasn't blocked.
	Remove(ctx context.Context, ip string) (removed bool, err error)
	// List returns every blocked {ip, peer_id} pair.
	List(ctx context.Context) ([]dto.BlacklistEntry, error)
	// Clear drops every entry (contracts.md: "the list ... is cleared when
	// the room closes").
	Clear(ctx context.Context) error
}

// BlacklistClearer drops every blacklist entry when the room closes.
// RoomUseCase depends on this (not BlackListStore directly) so it doesn't
// need to know about peer_id bookkeeping, only that the blacklist resets.
type BlacklistClearer interface {
	Clear(ctx context.Context) error
}

// EventPublisher pushes an event to every events stream open on a room
// (contracts.md GET /api/rooms/:room_id/events). Delivery is best effort and
// never blocks.
type EventPublisher interface {
	Publish(ctx context.Context, roomId string, ev domain.Event)
}

// EventSubscriber is the receiving side of EventPublisher, used by the
// events endpoint and the liveness watcher.
type EventSubscriber interface {
	// Subscribe opens a stream on roomId. cancel closes it.
	Subscribe(ctx context.Context, roomId string) (events <-chan domain.Event, cancel func())
	// Subscribers reports how many streams are open on roomId.
	Subscribers(ctx context.Context, roomId string) int
}

// StageReader builds the current stage of a room. LayoutUseCase implements
// it.
type StageReader interface {
	GetStage(ctx context.Context, roomId string) (dto.RoomLayoutQueryResponse, error)
}

// StageBroadcaster pushes the current stage to every events stream open on
// a room. LayoutUseCase implements it, so StreamerUseCase can announce a
// registration without knowing how the stage is built.
type StageBroadcaster interface {
	BroadcastStage(ctx context.Context, roomId string) error
}
