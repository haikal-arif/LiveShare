package usecase_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/usecase"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStageBroadcaster records the rooms it was asked to broadcast.
type fakeStageBroadcaster struct {
	rooms []string
	err   error
}

func (b *fakeStageBroadcaster) BroadcastStage(ctx context.Context, roomId string) error {
	b.rooms = append(b.rooms, roomId)
	return b.err
}

func TestStreamerUseCase_RegisterStreamer(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		streamerId  string
		req         dto.StreamerRegistrationRequest
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, got dto.StreamerRegistrationResponse, err error)
	}{
		{
			"registers a live streamer",
			"ABC-123",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "Hana A"},
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "streamer-1", got.StreamerID)
				assert.Equal(t, "Hana A", got.DisplayName)
				assert.True(t, got.Live)
			},
		},
		{
			"trims the display name",
			"ABC-123",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "  Hana A  "},
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "Hana A", got.DisplayName)
			},
		},
		{
			"blank display name",
			"ABC-123",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "   "},
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidDisplayName)
			},
		},
		{
			"display name over 40 characters",
			"ABC-123",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "0123456789012345678901234567890123456789x"},
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidDisplayName)
			},
		},
		{
			"peer not live",
			"ABC-123",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "Hana A"},
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrPeerNotLive)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			"streamer-1",
			dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: "Hana A"},
			func(store *repository.Store) *fakeRequestClient {
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.StreamerRegistrationResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			uc := usecase.NewStreamerUseCase(store, client, &fakeStageBroadcaster{})
			got, err := uc.RegisterStreamer(t.Context(), tt.roomId, tt.streamerId, tt.req)
			tt.checkResult(t, got, err)
		})
	}
}

func TestStreamerUseCase_RegisterStreamer_broadcastsStage(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		broadcaster *fakeStageBroadcaster
		checkResult func(t *testing.T, b *fakeStageBroadcaster, err error)
	}{
		{
			"announces a registration",
			"Hana A",
			&fakeStageBroadcaster{},
			func(t *testing.T, b *fakeStageBroadcaster, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"ABC-123"}, b.rooms)
			},
		},
		{
			"a failed broadcast doesn't fail the registration",
			"Hana A",
			&fakeStageBroadcaster{err: domain.ErrSFUUnreachable},
			func(t *testing.T, b *fakeStageBroadcaster, err error) {
				require.NoError(t, err)
			},
		},
		{
			"announces nothing when the registration is rejected",
			"   ",
			&fakeStageBroadcaster{},
			func(t *testing.T, b *fakeStageBroadcaster, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidDisplayName)
				assert.Empty(t, b.rooms)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			client := &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			uc := usecase.NewStreamerUseCase(store, client, tt.broadcaster)
			_, err := uc.RegisterStreamer(t.Context(), "ABC-123", "streamer-1", dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: tt.displayName})
			tt.checkResult(t, tt.broadcaster, err)
		})
	}
}

func TestStreamerUseCase_RegisterStreamer_displayNameLengthCountsCharacters(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantErr     error
	}{
		{"40 multi-byte characters are allowed", strings.Repeat("é", 40), nil},
		{"41 characters are too long", strings.Repeat("é", 41), domain.ErrInvalidDisplayName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			client := &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			uc := usecase.NewStreamerUseCase(store, client, &fakeStageBroadcaster{})
			_, err := uc.RegisterStreamer(t.Context(), "ABC-123", "streamer-1", dto.StreamerRegistrationRequest{PeerID: "pub-abc123", DisplayName: tt.displayName})
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
