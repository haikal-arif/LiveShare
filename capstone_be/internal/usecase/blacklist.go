package usecase

import (
	"context"
	"errors"
	"fmt"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

// BlacklistUseCase implements handler.BlacklistUseCase. It resolves a
// peer_id to an IP through the SFU, records the block in store (the same
// BlackListStore middleware.BlackListMiddleware guards requests with) and
// has the SFU disconnect the blocked peer.
type BlacklistUseCase struct {
	rooms  CurrentRoomProvider
	client RequestClient
	store  BlackListStore
}

func NewBlacklistUseCase(rooms CurrentRoomProvider, client RequestClient, store BlackListStore) *BlacklistUseCase {
	return &BlacklistUseCase{rooms: rooms, client: client, store: store}
}

func (uc *BlacklistUseCase) AddBlacklist(ctx context.Context, req dto.BlacklistAddRequest) (dto.BlacklistAddResponse, error) {
	roomId, ok := uc.rooms.Current(ctx)
	if !ok {
		return dto.BlacklistAddResponse{}, domain.ErrRoomNotFound
	}

	ip, err := uc.client.GetPeerIP(ctx, roomId, req.PeerID)
	if err != nil {
		if errors.Is(err, domain.ErrPeerNotFound) {
			return dto.BlacklistAddResponse{}, domain.ErrPeerNotFound
		}
		return dto.BlacklistAddResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}

	// Blocking an IP that's already blocked returns the existing entry;
	// BlackListStore.Add resolves that atomically.
	recordedPeerId, err := uc.store.Add(ctx, ip, req.PeerID)
	if err != nil {
		return dto.BlacklistAddResponse{}, fmt.Errorf("blacklist store: %w", err)
	}

	// Blocking also kicks the named peer (contracts.md POST /api/blacklist).
	// If this fails the block stays, so the admin can retry. A peer that's
	// already gone is fine: that's the outcome we wanted.
	if err := uc.client.DisconnectPeer(ctx, roomId, req.PeerID); err != nil && !errors.Is(err, domain.ErrPeerNotFound) {
		return dto.BlacklistAddResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}

	return dto.BlacklistAddResponse{RoomID: roomId, IP: ip, PeerID: recordedPeerId}, nil
}

func (uc *BlacklistUseCase) ListBlacklist(ctx context.Context) (dto.BlacklistListResponse, error) {
	roomId, ok := uc.rooms.Current(ctx)
	if !ok {
		return dto.BlacklistListResponse{}, domain.ErrRoomNotFound
	}

	entries, err := uc.store.List(ctx)
	if err != nil {
		return dto.BlacklistListResponse{}, fmt.Errorf("blacklist store: %w", err)
	}

	return dto.BlacklistListResponse{RoomID: roomId, Blacklist: entries}, nil
}

func (uc *BlacklistUseCase) RemoveBlacklist(ctx context.Context, ip string) error {
	if _, ok := uc.rooms.Current(ctx); !ok {
		return domain.ErrRoomNotFound
	}

	removed, err := uc.store.Remove(ctx, ip)
	if err != nil {
		return fmt.Errorf("blacklist store: %w", err)
	}
	if !removed {
		return domain.ErrNotBlacklisted
	}

	return nil
}

// Clear drops every blacklist entry. RoomUseCase.CloseRoom calls it:
// "the list lives in Go's memory and is cleared when the room closes."
func (uc *BlacklistUseCase) Clear(ctx context.Context) error {
	return uc.store.Clear(ctx)
}
