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

func TestBlacklistUseCase_AddBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		req         dto.BlacklistAddRequest
		peerIP      map[string]string
		peerIPErr   error
		setup       func(store *repository.Store, uc *usecase.BlacklistUseCase)
		checkResult func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error)
	}{
		{
			"blocks the resolved ip",
			dto.BlacklistAddRequest{PeerID: "pub-abc123"},
			map[string]string{"pub-abc123": "203.0.113.7"},
			nil,
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			},
			func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, dto.BlacklistAddResponse{RoomID: "ABC-123", IP: "203.0.113.7", PeerID: "pub-abc123"}, got)
				assert.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			"blocking an already-blocked ip returns the existing entry",
			dto.BlacklistAddRequest{PeerID: "pub-def456"},
			map[string]string{"pub-abc123": "203.0.113.7", "pub-def456": "203.0.113.7"},
			nil,
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
				_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-abc123"})
				require.NoError(t, err)
			},
			func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "203.0.113.7", got.IP)
				assert.Equal(t, "pub-abc123", got.PeerID)
			},
		},
		{
			"peer not found",
			dto.BlacklistAddRequest{PeerID: "pub-missing"},
			nil,
			domain.ErrPeerNotFound,
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			},
			func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrPeerNotFound)
			},
		},
		{
			"sfu unreachable",
			dto.BlacklistAddRequest{PeerID: "pub-abc123"},
			nil,
			assert.AnError,
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			},
			func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
			},
		},
		{
			"no room open",
			dto.BlacklistAddRequest{PeerID: "pub-abc123"},
			nil,
			nil,
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {},
			func(t *testing.T, list *repository.BlacklistStore, got dto.BlacklistAddResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			list := repository.NewBlacklistStore()
			client := &fakeRequestClient{peerIP: tt.peerIP, peerIPErr: tt.peerIPErr}
			uc := usecase.NewBlacklistUseCase(store, client, list)
			tt.setup(store, uc)
			got, err := uc.AddBlacklist(t.Context(), tt.req)
			tt.checkResult(t, list, got, err)
		})
	}
}

func TestBlacklistUseCase_AddBlacklist_disconnectsPeer(t *testing.T) {
	tests := []struct {
		name          string
		peerId        string
		disconnectErr error
		setup         func(store *repository.Store, uc *usecase.BlacklistUseCase, client *fakeRequestClient)
		checkResult   func(t *testing.T, list *repository.BlacklistStore, client *fakeRequestClient, err error)
	}{
		{
			"disconnects the blocked peer",
			"pub-abc123",
			nil,
			func(store *repository.Store, uc *usecase.BlacklistUseCase, client *fakeRequestClient) {},
			func(t *testing.T, list *repository.BlacklistStore, client *fakeRequestClient, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"pub-abc123"}, client.disconnected)
			},
		},
		{
			"blocking an already-blocked ip still disconnects the named peer",
			"sub-xyz789",
			nil,
			func(store *repository.Store, uc *usecase.BlacklistUseCase, client *fakeRequestClient) {
				_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-abc123"})
				require.NoError(t, err)
				client.disconnected = nil
			},
			func(t *testing.T, list *repository.BlacklistStore, client *fakeRequestClient, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"sub-xyz789"}, client.disconnected)
			},
		},
		{
			"peer that already left is not an error",
			"pub-abc123",
			domain.ErrPeerNotFound,
			func(store *repository.Store, uc *usecase.BlacklistUseCase, client *fakeRequestClient) {},
			func(t *testing.T, list *repository.BlacklistStore, client *fakeRequestClient, err error) {
				require.NoError(t, err)
				assert.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			"sfu failure keeps the block so the admin can retry",
			"pub-abc123",
			assert.AnError,
			func(store *repository.Store, uc *usecase.BlacklistUseCase, client *fakeRequestClient) {},
			func(t *testing.T, list *repository.BlacklistStore, client *fakeRequestClient, err error) {
				assert.ErrorIs(t, err, domain.ErrSFUUnreachable)
				assert.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			list := repository.NewBlacklistStore()
			client := &fakeRequestClient{
				peerIP:        map[string]string{"pub-abc123": "203.0.113.7", "sub-xyz789": "203.0.113.7"},
				disconnectErr: tt.disconnectErr,
			}
			uc := usecase.NewBlacklistUseCase(store, client, list)
			tt.setup(store, uc, client)
			_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: tt.peerId})
			tt.checkResult(t, list, client, err)
		})
	}
}

func TestBlacklistUseCase_AddBlacklist_unknownPeerIsNotDisconnected(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
	client := &fakeRequestClient{peerIPErr: domain.ErrPeerNotFound}
	uc := usecase.NewBlacklistUseCase(store, client, repository.NewBlacklistStore())

	_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-missing"})

	assert.ErrorIs(t, err, domain.ErrPeerNotFound)
	assert.Empty(t, client.disconnected)
}

func TestBlacklistUseCase_ListBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(store *repository.Store, uc *usecase.BlacklistUseCase)
		checkResult func(t *testing.T, got dto.BlacklistListResponse, err error)
	}{
		{
			"empty list",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			},
			func(t *testing.T, got dto.BlacklistListResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ABC-123", got.RoomID)
				assert.Equal(t, []dto.BlacklistEntry{}, got.Blacklist)
			},
		},
		{
			"one entry after a block",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
				_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-abc123"})
				require.NoError(t, err)
			},
			func(t *testing.T, got dto.BlacklistListResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, []dto.BlacklistEntry{{IP: "203.0.113.7", PeerID: "pub-abc123"}}, got.Blacklist)
			},
		},
		{
			"no room open",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {},
			func(t *testing.T, got dto.BlacklistListResponse, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			list := repository.NewBlacklistStore()
			client := &fakeRequestClient{peerIP: map[string]string{"pub-abc123": "203.0.113.7"}}
			uc := usecase.NewBlacklistUseCase(store, client, list)
			tt.setup(store, uc)
			got, err := uc.ListBlacklist(t.Context())
			tt.checkResult(t, got, err)
		})
	}
}

func TestBlacklistUseCase_RemoveBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		ip          string
		setup       func(store *repository.Store, uc *usecase.BlacklistUseCase)
		checkResult func(t *testing.T, list *repository.BlacklistStore, err error)
	}{
		{
			"unblocks a blocked ip",
			"203.0.113.7",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
				_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-abc123"})
				require.NoError(t, err)
			},
			func(t *testing.T, list *repository.BlacklistStore, err error) {
				require.NoError(t, err)
				assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			"ip not on the list",
			"203.0.113.9",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {
				require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
			},
			func(t *testing.T, list *repository.BlacklistStore, err error) {
				assert.ErrorIs(t, err, domain.ErrNotBlacklisted)
			},
		},
		{
			"no room open",
			"203.0.113.7",
			func(store *repository.Store, uc *usecase.BlacklistUseCase) {},
			func(t *testing.T, list *repository.BlacklistStore, err error) {
				assert.ErrorIs(t, err, domain.ErrRoomNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := repository.NewStore()
			list := repository.NewBlacklistStore()
			client := &fakeRequestClient{peerIP: map[string]string{"pub-abc123": "203.0.113.7"}}
			uc := usecase.NewBlacklistUseCase(store, client, list)
			tt.setup(store, uc)
			err := uc.RemoveBlacklist(t.Context(), tt.ip)
			tt.checkResult(t, list, err)
		})
	}
}

func TestBlacklistUseCase_Clear(t *testing.T) {
	store := repository.NewStore()
	require.NoError(t, store.Open(t.Context(), "ABC-123", "203.0.113.1"))
	list := repository.NewBlacklistStore()
	client := &fakeRequestClient{peerIP: map[string]string{"pub-abc123": "203.0.113.7"}}
	uc := usecase.NewBlacklistUseCase(store, client, list)

	_, err := uc.AddBlacklist(t.Context(), dto.BlacklistAddRequest{PeerID: "pub-abc123"})
	require.NoError(t, err)
	require.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))

	require.NoError(t, uc.Clear(t.Context()))

	assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
	got, err := uc.ListBlacklist(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []dto.BlacklistEntry{}, got.Blacklist)
}
