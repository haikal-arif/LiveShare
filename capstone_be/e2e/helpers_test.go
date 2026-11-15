//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"capstonewebserver/internal/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertError checks res is a contracts.md error body: the status and the
// machine-readable code.
func assertError(t *testing.T, res response, wantStatus int, wantCode string) {
	t.Helper()
	require.Equal(t, wantStatus, res.status, "body: %s", res.body)
	var got dto.ErrorResponse
	res.decode(t, &got)
	assert.Equal(t, wantCode, got.Error.Code)
	assert.NotEmpty(t, got.Error.Message)
}

func assertStatus(t *testing.T, res response, want int) {
	t.Helper()
	require.Equal(t, want, res.status, "body: %s", res.body)
}

// registerStreamer registers streamerID as peerID, which must already be a
// live publisher on the fake SFU.
func registerStreamer(t *testing.T, e env, streamerID, peerID, displayName string) {
	t.Helper()
	res := send(t, request{
		method: http.MethodPut,
		path:   "/api/rooms/" + e.roomID + "/streamers/" + streamerID,
		body:   `{"peer_id":"` + peerID + `","display_name":"` + displayName + `"}`,
	})
	assertStatus(t, res, http.StatusOK)
}
