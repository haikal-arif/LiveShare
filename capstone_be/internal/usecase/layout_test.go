package usecase_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/usecase"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutUseCase_GetStage(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, got dto.RoomLayoutQueryResponse, err error)
	}{
		{
			"no stage set yet",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, int64(0), got.Version)
				assert.Nil(t, got.Template)
				assert.Equal(t, []dto.Placement{}, got.Placements)
				assert.Equal(t, []dto.Streamer{}, got.Streamers)
			},
		},
		{
			"stage with a live and an offline streamer",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-2", "pub-def456", "User C"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				require.NoError(t, err)
				assert.Len(t, got.Streamers, 2)
				byID := map[string]dto.Streamer{}
				for _, s := range got.Streamers {
					byID[s.StreamerID] = s
				}
				assert.True(t, byID["streamer-1"].Live)
				assert.False(t, byID["streamer-2"].Live)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			func(store *repository.Store) *fakeRequestClient {
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			uc := usecase.NewLayoutUseCase(store, store, client, &fakeEventPublisher{})
			got, err := uc.GetStage(t.Context(), tt.roomId)
			tt.checkResult(t, got, err)
		})
	}
}

func TestLayoutUseCase_PutStage(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name        string
		roomId      string
		req         dto.RoomLayoutEditRequest
		setup       func(store *repository.Store)
		checkResult func(t *testing.T, got dto.RoomLayoutQueryResponse, err error)
	}{
		{
			"saves the stage",
			"ABC-123",
			dto.RoomLayoutEditRequest{
				Template:   strPtr("concert"),
				Placements: []dto.Placement{{StreamerID: "streamer-1", X: 0.3, Y: 0.2}},
			},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				require.NoError(t, err)
				// Registering streamer-1 bumped the version once; saving the
				// stage bumps it again.
				assert.Equal(t, int64(2), got.Version)
				assert.Equal(t, "concert", *got.Template)
				assert.Len(t, got.Placements, 1)
			},
		},
		{
			"null template removes the stage",
			"ABC-123",
			dto.RoomLayoutEditRequest{Template: nil, Placements: nil},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				require.NoError(t, err)
				assert.Nil(t, got.Template)
			},
		},
		{
			"invalid template",
			"ABC-123",
			dto.RoomLayoutEditRequest{Template: strPtr("castle")},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidTemplate)
			},
		},
		{
			"placements without template",
			"ABC-123",
			dto.RoomLayoutEditRequest{Template: nil, Placements: []dto.Placement{{StreamerID: "streamer-1"}}},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrPlacementsWithoutTemplate)
			},
		},
		{
			"out of range position",
			"ABC-123",
			dto.RoomLayoutEditRequest{Template: strPtr("concert"), Placements: []dto.Placement{{StreamerID: "streamer-1", X: 1.2, Y: 0.2}}},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidPosition)
			},
		},
		{
			"unknown streamer",
			"ABC-123",
			dto.RoomLayoutEditRequest{Template: strPtr("concert"), Placements: []dto.Placement{{StreamerID: "ghost", X: 0.1, Y: 0.1}}},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrUnknownStreamer)
			},
		},
		{
			"duplicate streamer",
			"ABC-123",
			dto.RoomLayoutEditRequest{
				Template: strPtr("concert"),
				Placements: []dto.Placement{
					{StreamerID: "streamer-1", X: 0.1, Y: 0.1},
					{StreamerID: "streamer-1", X: 0.2, Y: 0.2},
				},
			},
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
			},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrDuplicateStreamer)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			dto.RoomLayoutEditRequest{},
			func(store *repository.Store) {},
			func(t *testing.T, got dto.RoomLayoutQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			tt.setup(store)
			uc := usecase.NewLayoutUseCase(store, store, &fakeRequestClient{}, &fakeEventPublisher{})
			got, err := uc.PutStage(t.Context(), tt.roomId, tt.req)
			tt.checkResult(t, got, err)
		})
	}
}

func TestLayoutUseCase_PutStage_publishesStage(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
	events := &fakeEventPublisher{}
	uc := usecase.NewLayoutUseCase(store, store, &fakeRequestClient{}, events)
	template := "concert"

	got, err := uc.PutStage(t.Context(), "ABC-123", dto.RoomLayoutEditRequest{Template: &template})
	require.NoError(t, err)

	assert.Equal(t, []domain.Event{{Name: domain.EventStageUpdated, Data: got}}, events.published)
}

func TestLayoutUseCase_PutStage_invalidRequestPublishesNothing(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
	events := &fakeEventPublisher{}
	uc := usecase.NewLayoutUseCase(store, store, &fakeRequestClient{}, events)
	template := "stadium"

	_, err := uc.PutStage(t.Context(), "ABC-123", dto.RoomLayoutEditRequest{Template: &template})

	assert.ErrorIs(t, err, domain.ErrInvalidTemplate)
	assert.Empty(t, events.published)
}

func TestLayoutUseCase_BroadcastStage(t *testing.T) {
	tests := []struct {
		name        string
		client      *fakeRequestClient
		checkResult func(t *testing.T, events *fakeEventPublisher, err error)
	}{
		{
			"publishes the current stage with liveness",
			&fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}},
			func(t *testing.T, events *fakeEventPublisher, err error) {
				require.NoError(t, err)
				require.Len(t, events.published, 1)
				assert.Equal(t, domain.EventStageUpdated, events.published[0].Name)
				stage, ok := events.published[0].Data.(dto.RoomLayoutQueryResponse)
				require.True(t, ok)
				assert.Equal(t, []dto.Streamer{{StreamerID: "streamer-1", PeerID: "pub-abc123", DisplayName: "Hana A", Live: true}}, stage.Streamers)
			},
		},
		{
			"sfu unreachable publishes nothing",
			&fakeRequestClient{publishersErr: assert.AnError},
			func(t *testing.T, events *fakeEventPublisher, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
				assert.Empty(t, events.published)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			require.NoError(t, store.Upsert(t.Context(), "streamer-1", "pub-abc123", "Hana A"))
			events := &fakeEventPublisher{}
			uc := usecase.NewLayoutUseCase(store, store, tt.client, events)
			err := uc.BroadcastStage(t.Context(), "ABC-123")
			tt.checkResult(t, events, err)
		})
	}
}
