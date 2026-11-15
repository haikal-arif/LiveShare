package usecase_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/events"
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/usecase"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsUseCase_Subscribe(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, hub *events.Hub, initial dto.RoomLayoutQueryResponse, err error)
	}{
		{
			"returns the current stage and keeps the stream open",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, hub *events.Hub, initial dto.RoomLayoutQueryResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ABC-123", initial.RoomID)
				assert.Equal(t, 1, hub.Subscribers(t.Context(), "ABC-123"))
			},
		},
		{
			"room not found",
			"ZZZ-999",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, hub *events.Hub, initial dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
				assert.Equal(t, 0, hub.Subscribers(t.Context(), "ZZZ-999"))
			},
		},
		{
			"sfu unreachable doesn't leave a stream open",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishersErr: assert.AnError}
			},
			func(t *testing.T, hub *events.Hub, initial dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
				assert.Equal(t, 0, hub.Subscribers(t.Context(), "ABC-123"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			hub := events.NewHub()
			layout := usecase.NewLayoutUseCase(store, store, client, hub)
			uc := usecase.NewEventsUseCase(store, hub, layout)
			initial, _, _, err := uc.Subscribe(t.Context(), tt.roomId)
			tt.checkResult(t, hub, initial, err)
		})
	}
}

func TestEventsUseCase_Subscribe_receivesLaterEvents(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
	hub := events.NewHub()
	layout := usecase.NewLayoutUseCase(store, store, &fakeRequestClient{}, hub)
	uc := usecase.NewEventsUseCase(store, hub, layout)
	_, stream, cancel, err := uc.Subscribe(t.Context(), "ABC-123")
	require.NoError(t, err)
	defer cancel()
	template := "concert"

	_, err = layout.PutStage(t.Context(), "ABC-123", dto.RoomLayoutEditRequest{Template: &template})
	require.NoError(t, err)

	ev := <-stream
	assert.Equal(t, domain.EventStageUpdated, ev.Name)
}
