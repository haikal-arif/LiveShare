package usecase

import (
	"context"
	"maps"
	"time"

	"capstonewebserver/internal/domain"
)

// LivenessUseCase notices streamers going live or offline and pushes a
// stage_updated for it (contracts.md decision 11). Only the SFU knows who is
// connected, so Go polls it, and only while someone is listening.
type LivenessUseCase struct {
	rooms  CurrentRoomProvider
	subs   EventSubscriber
	stages StageReader
	events EventPublisher

	lastRoom string
	last     map[string]bool // streamer_id -> live at the last push; nil = no baseline
}

func NewLivenessUseCase(rooms CurrentRoomProvider, subs EventSubscriber, stages StageReader, events EventPublisher) *LivenessUseCase {
	return &LivenessUseCase{rooms: rooms, subs: subs, stages: stages, events: events}
}

// Run calls Poll every interval until ctx is done.
func (uc *LivenessUseCase) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			uc.Poll(ctx)
		}
	}
}

// Poll checks the open room once. It's meant for a single goroutine (Run).
func (uc *LivenessUseCase) Poll(ctx context.Context) {
	roomId, ok := uc.rooms.Current(ctx)
	if !ok || uc.subs.Subscribers(ctx, roomId) == 0 {
		uc.last = nil
		return
	}
	if roomId != uc.lastRoom {
		uc.lastRoom, uc.last = roomId, nil
	}

	stage, err := uc.stages.GetStage(ctx, roomId)
	if err != nil {
		// SFU down or room just closed: nothing to push, try again next tick.
		return
	}

	live := make(map[string]bool, len(stage.Streamers))
	for _, s := range stage.Streamers {
		live[s.StreamerID] = s.Live
	}
	// With no baseline the first poll always pushes: a change could have
	// slipped in between a stream's initial stage and now.
	if uc.last != nil && maps.Equal(live, uc.last) {
		return
	}
	uc.last = live

	uc.events.Publish(ctx, roomId, domain.Event{Name: domain.EventStageUpdated, Data: stage})
}
