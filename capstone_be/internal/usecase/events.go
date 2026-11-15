package usecase

import (
	"context"
	"fmt"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

// EventsUseCase implements handler.EventsUseCase (contracts.md GET
// /api/rooms/:room_id/events).
type EventsUseCase struct {
	rs     RoomStore
	subs   EventSubscriber
	stages StageReader
}

func NewEventsUseCase(rs RoomStore, subs EventSubscriber, stages StageReader) *EventsUseCase {
	return &EventsUseCase{rs: rs, subs: subs, stages: stages}
}

// Subscribe opens an events stream on roomId. initial is the stage the
// stream must send first, so a client needs no separate GET .../stage; events
// carries everything after it. cancel must be called when the stream ends.
func (uc *EventsUseCase) Subscribe(ctx context.Context, roomId string) (initial dto.RoomLayoutQueryResponse, events <-chan domain.Event, cancel func(), err error) {
	exist, err := uc.rs.IsExist(ctx, roomId)
	if err != nil {
		return dto.RoomLayoutQueryResponse{}, nil, nil, fmt.Errorf("room store: %w", err)
	}
	if !exist {
		return dto.RoomLayoutQueryResponse{}, nil, nil, domain.ErrRoomNotFound
	}

	// Subscribe before reading the stage so a change landing in between is
	// queued rather than lost; clients skip any event older than the version
	// they have.
	events, cancel = uc.subs.Subscribe(ctx, roomId)
	initial, err = uc.stages.GetStage(ctx, roomId)
	if err != nil {
		cancel()
		return dto.RoomLayoutQueryResponse{}, nil, nil, err
	}
	return initial, events, cancel, nil
}
