package repository_test

import (
	"capstonewebserver/internal/repository"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlacklistStore(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, list *repository.BlacklistStore)
		checkResult func(t *testing.T, list *repository.BlacklistStore)
	}{
		{
			name: "ip not blocked by default",
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			name: "adding blocks the ip",
			setup: func(t *testing.T, list *repository.BlacklistStore) {
				_, err := list.Add(t.Context(), "203.0.113.7", "pub-abc123")
				require.NoError(t, err)
			},
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			name: "adding twice returns the existing peer_id",
			setup: func(t *testing.T, list *repository.BlacklistStore) {
				_, err := list.Add(t.Context(), "203.0.113.7", "pub-abc123")
				require.NoError(t, err)
				got, err := list.Add(t.Context(), "203.0.113.7", "pub-def456")
				require.NoError(t, err)
				assert.Equal(t, "pub-abc123", got)
			},
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.True(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			name: "removing unblocks the ip",
			setup: func(t *testing.T, list *repository.BlacklistStore) {
				_, err := list.Add(t.Context(), "203.0.113.7", "pub-abc123")
				require.NoError(t, err)
				removed, err := list.Remove(t.Context(), "203.0.113.7")
				require.NoError(t, err)
				assert.True(t, removed)
			},
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			name: "removing an ip that was never blocked reports not-removed",
			setup: func(t *testing.T, list *repository.BlacklistStore) {
				removed, err := list.Remove(t.Context(), "203.0.113.7")
				require.NoError(t, err)
				assert.False(t, removed)
			},
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
			},
		},
		{
			name: "clear drops every entry",
			setup: func(t *testing.T, list *repository.BlacklistStore) {
				_, err := list.Add(t.Context(), "203.0.113.7", "pub-abc123")
				require.NoError(t, err)
				require.NoError(t, list.Clear(t.Context()))
			},
			checkResult: func(t *testing.T, list *repository.BlacklistStore) {
				assert.False(t, list.IsBlocked(t.Context(), "203.0.113.7"))
				entries, err := list.List(t.Context())
				require.NoError(t, err)
				assert.Equal(t, 0, len(entries))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := repository.NewBlacklistStore()
			if tt.setup != nil {
				tt.setup(t, list)
			}
			tt.checkResult(t, list)
		})
	}
}
