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

// livenessSetup is a room with one registered streamer and an open events
// stream, so the watcher has someone to poll for.
func livenessSetup(t *testing.T, client *fakeRequestClient) (*usecase.LivenessUseCase, *fakeEventPublisher, func()) {
	t.Helper()
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
	require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
	hub := events.NewHub()
	_, cancel := hub.Subscribe(t.Context(), "ABC-123")
	layout := usecase.NewLayoutUseCase(store, store, client, hub)
	published := &fakeEventPublisher{}
	return usecase.NewLivenessUseCase(store, hub, layout, published), published, cancel
}

func TestLivenessUseCase_Poll(t *testing.T) {
	tests := []struct {
		name        string
		run         func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient)
		wantPushes  int
		checkResult func(t *testing.T, published []domain.Event)
	}{
		{
			"first poll pushes the stage",
			func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient) {
				uc.Poll(t.Context())
			},
			1,
			func(t *testing.T, published []domain.Event) {
				assert.Equal(t, domain.EventStageUpdated, published[0].Name)
			},
		},
		{
			"nothing changed means nothing pushed",
			func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient) {
				uc.Poll(t.Context())
				uc.Poll(t.Context())
				uc.Poll(t.Context())
			},
			1,
			nil,
		},
		{
			"a streamer going live is pushed",
			func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient) {
				uc.Poll(t.Context())
				client.publishers = []dto.Publisher{{PeerID: "pub-abc123"}}
				uc.Poll(t.Context())
			},
			2,
			func(t *testing.T, published []domain.Event) {
				stage := published[1].Data.(dto.RoomLayoutQueryResponse)
				assert.True(t, stage.Streamers[0].Live)
			},
		},
		{
			"a streamer going offline is pushed",
			func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient) {
				client.publishers = []dto.Publisher{{PeerID: "pub-abc123"}}
				uc.Poll(t.Context())
				client.publishers = nil
				uc.Poll(t.Context())
			},
			2,
			func(t *testing.T, published []domain.Event) {
				stage := published[1].Data.(dto.RoomLayoutQueryResponse)
				assert.False(t, stage.Streamers[0].Live)
			},
		},
		{
			"sfu unreachable pushes nothing",
			func(t *testing.T, uc *usecase.LivenessUseCase, client *fakeRequestClient) {
				client.publishersErr = assert.AnError
				uc.Poll(t.Context())
			},
			0,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeRequestClient{}
			uc, published, cancel := livenessSetup(t, client)
			defer cancel()
			tt.run(t, uc, client)
			require.Len(t, published.published, tt.wantPushes)
			if tt.checkResult != nil {
				tt.checkResult(t, published.published)
			}
		})
	}
}

func TestLivenessUseCase_Poll_skipsWhenNobodyIsListening(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
	hub := events.NewHub()
	client := &fakeRequestClient{publishersErr: assert.AnError}
	layout := usecase.NewLayoutUseCase(store, store, client, hub)
	published := &fakeEventPublisher{}
	uc := usecase.NewLivenessUseCase(store, hub, layout, published)

	uc.Poll(t.Context())

	assert.Empty(t, published.published)
}

func TestLivenessUseCase_Poll_resendsAfterEveryoneLeft(t *testing.T) {
	client := &fakeRequestClient{}
	uc, published, cancel := livenessSetup(t, client)
	uc.Poll(t.Context())
	cancel() // last stream closes; the watcher forgets its baseline
	uc.Poll(t.Context())
	require.Len(t, published.published, 1)
}
