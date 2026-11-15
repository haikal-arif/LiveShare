package usecase

import (
	"context"
	"fmt"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

var validTemplates = map[string]bool{"concert": true, "conference": true, "theater": true, "blank": true}

// LayoutUseCase implements handler.LayoutUseCase.
type LayoutUseCase struct {
	ss        StageStore
	streamers StreamerStore
	client    RequestClient
	events    EventPublisher
}

func NewLayoutUseCase(ss StageStore, streamers StreamerStore, client RequestClient, events EventPublisher) *LayoutUseCase {
	return &LayoutUseCase{ss: ss, streamers: streamers, client: client, events: events}
}

func (uc *LayoutUseCase) GetStage(ctx context.Context, roomId string) (dto.RoomLayoutQueryResponse, error) {
	exist, err := uc.ss.RoomExists(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("stage store: %w", err)
	}
	if !exist {
		return dto.RoomLayoutQueryResponse{}, domain.ErrRoomNotFound
	}

	return uc.buildStage(ctx, roomId)
}

func (uc *LayoutUseCase) PutStage(ctx context.Context, roomId string, req dto.RoomLayoutEditRequest) (dto.RoomLayoutQueryResponse, error) {
	exist, err := uc.ss.RoomExists(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("stage store: %w", err)
	}
	if !exist {
		return dto.RoomLayoutQueryResponse{}, domain.ErrRoomNotFound
	}

	if req.Template == nil {
		if len(req.Placements) > 0 {
			return dto.RoomLayoutQueryResponse{}, domain.ErrPlacementsWithoutTemplate
		}
	} else if !validTemplates[*req.Template] {
		return dto.RoomLayoutQueryResponse{}, domain.ErrInvalidTemplate
	}

	seen := make(map[string]bool, len(req.Placements))
	for _, p := range req.Placements {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			return dto.RoomLayoutQueryResponse{}, domain.ErrInvalidPosition
		}
		known, err := uc.streamers.Exists(ctx, p.StreamerID)
		if err != nil {
			return dto.RoomLayoutQueryResponse{}, fmt.Errorf("streamer store: %w", err)
		}
		if !known {
			return dto.RoomLayoutQueryResponse{}, domain.ErrUnknownStreamer
		}
		if seen[p.StreamerID] {
			return dto.RoomLayoutQueryResponse{}, domain.ErrDuplicateStreamer
		}
		seen[p.StreamerID] = true
	}

	if err := uc.ss.SetRoomLayout(ctx, roomId, domain.RoomLayout{Template: req.Template, Placements: req.Placements}); err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("stage store: %w", err)
	}

	stage, err := uc.buildStage(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, err
	}
	uc.events.Publish(ctx, roomId, domain.Event{Name: domain.EventStageUpdated, Data: stage})
	return stage, nil
}

// BroadcastStage pushes the current stage as a stage_updated event to every
// events stream open on roomId.
func (uc *LayoutUseCase) BroadcastStage(ctx context.Context, roomId string) error {
	stage, err := uc.buildStage(ctx, roomId)
	if err != nil {
		return err
	}
	uc.events.Publish(ctx, roomId, domain.Event{Name: domain.EventStageUpdated, Data: stage})
	return nil
}

// buildStage composes the full stage response: template, placements and
// every registered streamer with its current liveness (contracts.md
// GET /api/rooms/:room_id/stage).
func (uc *LayoutUseCase) buildStage(ctx context.Context, roomId string) (dto.RoomLayoutQueryResponse, error) {
	layout, err := uc.ss.GetRoomLayout(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("stage store: %w", err)
	}

	registered, err := uc.streamers.List(ctx)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("streamer store: %w", err)
	}

	publishers, err := uc.client.GetPublishers(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}
	live := make(map[string]bool, len(publishers))
	for _, p := range publishers {
		live[p.PeerID] = true
	}

	streamers := make([]dto.Streamer, len(registered))
	for i, s := range registered {
		s.Live = live[s.PeerID]
		streamers[i] = s
	}

	placements := layout.Placements
	if placements == nil {
		placements = []dto.Placement{}
	}

	return dto.RoomLayoutQueryResponse{
		RoomID:     roomId,
		Version:    layout.Version,
		Template:   layout.Template,
		Placements: placements,
		Streamers:  streamers,
	}, nil
}
