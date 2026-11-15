package handler

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"context"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=handler.go -destination=mock_room_usecase_test.go -package=handler

type RoomUseCase interface {
	CreateRoom(ctx context.Context, passcode string) (dto.RoomCreationResponse, error)
	RoomDetail(ctx context.Context, roomId string) (dto.RoomQueryResponse, error)
	RoomPublishers(ctx context.Context, roomId string) (dto.RoomPublisherQueryResponse, error)
	ReclaimRoom(ctx context.Context, roomId string, ip string) (dto.RoomReclaimResponse, error)
	CloseRoom(ctx context.Context, roomId string) error
}

// StreamerUseCase backs the streamer registry endpoints (contracts.md
// "Streamers").
type StreamerUseCase interface {
	RegisterStreamer(ctx context.Context, roomId, streamerId string, req dto.StreamerRegistrationRequest) (dto.StreamerRegistrationResponse, error)
}

// LayoutUseCase backs the stage layout endpoints (contracts.md
// "Stage layout").
type LayoutUseCase interface {
	GetStage(ctx context.Context, roomId string) (dto.RoomLayoutQueryResponse, error)
	PutStage(ctx context.Context, roomId string, req dto.RoomLayoutEditRequest) (dto.RoomLayoutQueryResponse, error)
}

// BlacklistUseCase backs the /api/blacklist endpoints.
type BlacklistUseCase interface {
	AddBlacklist(ctx context.Context, req dto.BlacklistAddRequest) (dto.BlacklistAddResponse, error)
	ListBlacklist(ctx context.Context) (dto.BlacklistListResponse, error)
	RemoveBlacklist(ctx context.Context, ip string) error
}

// EventsUseCase backs GET /api/rooms/:room_id/events (contracts.md "events").
type EventsUseCase interface {
	// Subscribe returns the stage to send first and the events that follow
	// it. cancel must be called when the stream ends.
	Subscribe(ctx context.Context, roomId string) (initial dto.RoomLayoutQueryResponse, events <-chan domain.Event, cancel func(), err error)
}

type WebServerHandler struct {
	ruc RoomUseCase
	suc StreamerUseCase
	luc LayoutUseCase
	buc BlacklistUseCase
	euc EventsUseCase
}

func NewWebServerHandler(ruc RoomUseCase, suc StreamerUseCase, luc LayoutUseCase, buc BlacklistUseCase, euc EventsUseCase) WebServerHandler {
	return WebServerHandler{ruc: ruc, suc: suc, luc: luc, buc: buc, euc: euc}
}
