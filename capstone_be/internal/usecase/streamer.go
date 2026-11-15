package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

// StreamerUseCase implements handler.StreamerUseCase.
type StreamerUseCase struct {
	store  StreamerStore
	client RequestClient
	stages StageBroadcaster
}

func NewStreamerUseCase(store StreamerStore, client RequestClient, stages StageBroadcaster) *StreamerUseCase {
	return &StreamerUseCase{store: store, client: client, stages: stages}
}

func (uc *StreamerUseCase) RegisterStreamer(ctx context.Context, roomId, streamerId string, req dto.StreamerRegistrationRequest) (dto.StreamerRegistrationResponse, error) {
	exist, err := uc.store.RoomExists(ctx, roomId)
	if err != nil {
		return dto.StreamerRegistrationResponse{}, fmt.Errorf("streamer store: %w", err)
	}
	if !exist {
		return dto.StreamerRegistrationResponse{}, domain.ErrRoomNotFound
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if n := utf8.RuneCountInString(displayName); n < 1 || n > 40 {
		return dto.StreamerRegistrationResponse{}, domain.ErrInvalidDisplayName
	}

	publishers, err := uc.client.GetPublishers(ctx, roomId)
	if err != nil {
		return dto.StreamerRegistrationResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}
	live := false
	for _, p := range publishers {
		if p.PeerID == req.PeerID {
			live = true
			break
		}
	}
	if !live {
		return dto.StreamerRegistrationResponse{}, domain.ErrPeerNotLive
	}

	if err := uc.store.Upsert(ctx, streamerId, req.PeerID, displayName); err != nil {
		return dto.StreamerRegistrationResponse{}, fmt.Errorf("streamer store: %w", err)
	}

	// The registration is already stored, so a failure to announce it (the
	// SFU just stopped answering) doesn't fail the call; the liveness
	// watcher pushes the stage again on its next change.
	_ = uc.stages.BroadcastStage(ctx, roomId)

	return dto.Streamer{StreamerID: streamerId, PeerID: req.PeerID, DisplayName: displayName, Live: true}, nil
}
