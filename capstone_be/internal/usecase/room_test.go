package usecase_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/reqctx"
	"capstonewebserver/internal/usecase"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRequestClient struct {
	publishers    []dto.Publisher
	publishersErr error
	peerIP        map[string]string
	peerIPErr     error
	closeErr      error

	disconnectErr error
	disconnected  []string // peer_ids passed to DisconnectPeer
}

func (c *fakeRequestClient) GetPublishers(ctx context.Context, roomId string) ([]dto.Publisher, error) {
	return c.publishers, c.publishersErr
}

func (c *fakeRequestClient) GetPeerIP(ctx context.Context, roomId, peerId string) (string, error) {
	if c.peerIPErr != nil {
		return "", c.peerIPErr
	}
	return c.peerIP[peerId], nil
}

func (c *fakeRequestClient) DisconnectPeer(ctx context.Context, roomId, peerId string) error {
	c.disconnected = append(c.disconnected, peerId)
	return c.disconnectErr
}

func (c *fakeRequestClient) CloseRoom(ctx context.Context, roomId string) error {
	return c.closeErr
}

// fakeEventPublisher records what was published so a test can check it.
type fakeEventPublisher struct {
	published []domain.Event
}

func (p *fakeEventPublisher) Publish(ctx context.Context, roomId string, ev domain.Event) {
	p.published = append(p.published, ev)
}

type fakeBlacklistClearer struct {
	cleared  bool
	clearErr error
}

func (b *fakeBlacklistClearer) Clear(ctx context.Context) error {
	if b.clearErr != nil {
		return b.clearErr
	}
	b.cleared = true
	return nil
}

func TestRoomUseCase_CreateRoom(t *testing.T) {
	tests := []struct {
		name        string
		passcode    string
		setup       func(store *repository.Store)
		checkResult func(t *testing.T, got dto.RoomCreationResponse, err error)
	}{
		{
			"wrong passcode",
			"nope",
			nil,
			func(t *testing.T, got dto.RoomCreationResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrInvalidPasscode)
			},
		},
		{
			"room created",
			"the-passcode-from-env",
			nil,
			func(t *testing.T, got dto.RoomCreationResponse, err error) {
				require.NoError(t, err)
				assert.Regexp(t, `^[A-HJ-NP-Z2-9]{3}-[A-HJ-NP-Z2-9]{3}$`, got.RoomID)
				claims, err := usecase.VerifyAdminToken(got.AdminToken, []byte("secret"))
				require.NoError(t, err)
				assert.True(t, claims.Admin)
				assert.Equal(t, got.RoomID, claims.RoomID)
			},
		},
		{
			"room already open",
			"the-passcode-from-env",
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomCreationResponse, err error) {
				var exists *domain.RoomExistsError
				require.ErrorAs(t, err, &exists)
				assert.Equal(t, "ABC-123", exists.RoomID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			if tt.setup != nil {
				tt.setup(store)
			}
			uc := usecase.NewRoomUseCase(store, &fakeRequestClient{}, &fakeBlacklistClearer{}, &fakeEventPublisher{}, usecase.RoomConfig{
				Secret:   []byte("secret"),
				Passcode: "the-passcode-from-env",
			})
			got, err := uc.CreateRoom(t.Context(), tt.passcode)
			tt.checkResult(t, got, err)
		})
	}
}

func TestRoomUseCase_CreateRoom_recordsCreatorIP(t *testing.T) {
	store := repository.NewStore()
	uc := usecase.NewRoomUseCase(store, &fakeRequestClient{}, &fakeBlacklistClearer{}, &fakeEventPublisher{}, usecase.RoomConfig{
		Secret:   []byte("secret"),
		Passcode: "the-passcode-from-env",
	})

	ctx := reqctx.WithIP(t.Context(), "203.0.113.7")
	got, err := uc.CreateRoom(ctx, "the-passcode-from-env")
	require.NoError(t, err)

	owner, err := store.OwnerIp(t.Context(), got.RoomID)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.7", owner)
}

func TestRoomUseCase_RoomDetail(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, got dto.RoomQueryResponse, err error)
	}{
		{
			"existing room",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}}}
			},
			func(t *testing.T, got dto.RoomQueryResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ABC-123", got.RoomID)
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-abc123"}}, got.Publishers)
			},
		},
		{
			"empty room still returns an empty, non-nil publishers list",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.RoomQueryResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, []dto.Publisher{}, got.Publishers)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			func(store *repository.Store) *fakeRequestClient {
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.RoomQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
		{
			"sfu unreachable",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishersErr: assert.AnError}
			},
			func(t *testing.T, got dto.RoomQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			uc := usecase.NewRoomUseCase(store, client, &fakeBlacklistClearer{}, &fakeEventPublisher{}, usecase.RoomConfig{})
			got, err := uc.RoomDetail(t.Context(), tt.roomId)
			tt.checkResult(t, got, err)
		})
	}
}

func TestRoomUseCase_RoomPublishers(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, got dto.RoomPublisherQueryResponse, err error)
	}{
		{
			"existing room",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{publishers: []dto.Publisher{{PeerID: "pub-abc123"}, {PeerID: "pub-def456"}}}
			},
			func(t *testing.T, got dto.RoomPublisherQueryResponse, err error) {
				require.NoError(t, err)
				assert.Len(t, got.Publishers, 2)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			func(store *repository.Store) *fakeRequestClient {
				return &fakeRequestClient{}
			},
			func(t *testing.T, got dto.RoomPublisherQueryResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			uc := usecase.NewRoomUseCase(store, client, &fakeBlacklistClearer{}, &fakeEventPublisher{}, usecase.RoomConfig{})
			got, err := uc.RoomPublishers(t.Context(), tt.roomId)
			tt.checkResult(t, got, err)
		})
	}
}

func TestRoomUseCase_ReclaimRoom(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		ip          string
		setup       func(store *repository.Store)
		checkResult func(t *testing.T, got dto.RoomReclaimResponse, err error)
	}{
		{
			"same ip reclaims",
			"ABC-123",
			"203.0.113.7",
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomReclaimResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ABC-123", got.RoomID)
				claims, err := usecase.VerifyAdminToken(got.AdminToken, []byte("secret"))
				require.NoError(t, err)
				assert.Equal(t, "ABC-123", claims.RoomID)
			},
		},
		{
			"different ip rejected",
			"ABC-123",
			"203.0.113.9",
			func(store *repository.Store) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
			},
			func(t *testing.T, got dto.RoomReclaimResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrIPMismatch)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			"203.0.113.7",
			func(store *repository.Store) {},
			func(t *testing.T, got dto.RoomReclaimResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			tt.setup(store)
			uc := usecase.NewRoomUseCase(store, &fakeRequestClient{}, &fakeBlacklistClearer{}, &fakeEventPublisher{}, usecase.RoomConfig{Secret: []byte("secret")})
			got, err := uc.ReclaimRoom(t.Context(), tt.roomId, tt.ip)
			tt.checkResult(t, got, err)
		})
	}
}

func TestRoomUseCase_CloseRoom(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(store *repository.Store) *fakeRequestClient
		checkResult func(t *testing.T, store *repository.Store, blacklist *fakeBlacklistClearer, events *fakeEventPublisher, err error)
	}{
		{
			"closes the room and clears the blacklist",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, store *repository.Store, blacklist *fakeBlacklistClearer, events *fakeEventPublisher, err error) {
				require.NoError(t, err)
				exist, err := store.IsExist(t.Context(), "ABC-123")
				require.NoError(t, err)
				assert.False(t, exist)
				assert.True(t, blacklist.cleared)
			},
		},
		{
			"announces room_closed on the events streams",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{}
			},
			func(t *testing.T, store *repository.Store, blacklist *fakeBlacklistClearer, events *fakeEventPublisher, err error) {
				require.NoError(t, err)
				assert.Equal(t, []domain.Event{{Name: domain.EventRoomClosed, Data: dto.RoomClosedEvent{RoomID: "ABC-123"}}}, events.published)
			},
		},
		{
			"room not found",
			"ZZZ-999",
			func(store *repository.Store) *fakeRequestClient {
				return &fakeRequestClient{}
			},
			func(t *testing.T, store *repository.Store, blacklist *fakeBlacklistClearer, events *fakeEventPublisher, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
				assert.False(t, blacklist.cleared)
				assert.Empty(t, events.published)
			},
		},
		{
			"sfu unreachable leaves the room open",
			"ABC-123",
			func(store *repository.Store) *fakeRequestClient {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.7"))
				return &fakeRequestClient{closeErr: assert.AnError}
			},
			func(t *testing.T, store *repository.Store, blacklist *fakeBlacklistClearer, events *fakeEventPublisher, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
				exist, err := store.IsExist(t.Context(), "ABC-123")
				require.NoError(t, err)
				assert.True(t, exist)
				assert.False(t, blacklist.cleared)
				assert.Empty(t, events.published)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			client := tt.setup(store)
			blacklist := &fakeBlacklistClearer{}
			events := &fakeEventPublisher{}
			uc := usecase.NewRoomUseCase(store, client, blacklist, events, usecase.RoomConfig{})
			err := uc.CloseRoom(t.Context(), tt.roomId)
			tt.checkResult(t, store, blacklist, events, err)
		})
	}
}
