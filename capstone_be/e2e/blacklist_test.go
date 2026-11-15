//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"capstonewebserver/internal/dto"

	"github.com/stretchr/testify/assert"
)

// blockIP makes peerID resolve to ip on the fake SFU and blacklists it.
func blockIP(t *testing.T, e env, peerID, ip string) {
	t.Helper()
	sfu.setPeerIP(e.roomID, peerID, ip)
	res := send(t, request{method: http.MethodPost, path: "/api/blacklist", token: e.token, body: `{"peer_id":"` + peerID + `"}`})
	assertStatus(t, res, http.StatusOK)
}

func TestAddBlacklist_disconnectsPeer(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, e env)
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"the blocked peer is disconnected and drops out of the publisher list",
			func(t *testing.T, e env) {
				sfu.setPublishers(e.roomID, "pub-abc123", "pub-def456")
				sfu.setPeerIP(e.roomID, "pub-abc123", "203.0.113.7")
			},
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				assert.True(t, sfu.wasDisconnected(e.roomID, "pub-abc123"))

				list := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID + "/publishers"})
				assertStatus(t, list, http.StatusOK)
				var got dto.RoomPublisherQueryResponse
				list.decode(t, &got)
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-def456"}}, got.Publishers)
			},
		},
		{
			"an sfu failure on disconnect answers 502 but keeps the block",
			func(t *testing.T, e env) {
				sfu.setPeerIP(e.roomID, "pub-abc123", "203.0.113.7")
				sfu.setFailDisconnect(true)
			},
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")

				list := send(t, request{method: http.MethodGet, path: "/api/blacklist", token: e.token})
				assertStatus(t, list, http.StatusOK)
				var got dto.BlacklistListResponse
				list.decode(t, &got)
				assert.Equal(t, []dto.BlacklistEntry{{IP: "203.0.113.7", PeerID: "pub-abc123"}}, got.Blacklist)
			},
		},
		{
			"the admin can retry after the sfu recovers",
			func(t *testing.T, e env) {
				sfu.setPeerIP(e.roomID, "pub-abc123", "203.0.113.7")
				sfu.setFailDisconnect(true)
				res := send(t, request{method: http.MethodPost, path: "/api/blacklist", token: e.token, body: `{"peer_id":"pub-abc123"}`})
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")
				sfu.setFailDisconnect(false)
			},
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				assert.True(t, sfu.wasDisconnected(e.roomID, "pub-abc123"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := openRoom(t)
			tt.setup(t, e)
			res := send(t, request{method: http.MethodPost, path: "/api/blacklist", token: e.token, body: `{"peer_id":"pub-abc123"}`})
			tt.checkResult(t, e, res)
		})
	}
}

func TestAddBlacklist(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		body        string
		setup       func(t *testing.T) env
		token       func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"admin blocks a peer's ip",
			`{"peer_id":"pub-abc123"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPeerIP(e.roomID, "pub-abc123", "203.0.113.7")
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.BlacklistAddResponse
				res.decode(t, &got)
				assert.Equal(t, dto.BlacklistAddResponse{RoomID: e.roomID, IP: "203.0.113.7", PeerID: "pub-abc123"}, got)
			},
		},
		{
			"blocking an already blocked ip returns the existing entry",
			`{"peer_id":"sub-xyz789"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPeerIP(e.roomID, "sub-xyz789", "203.0.113.7")
				blockIP(t, e, "pub-abc123", "203.0.113.7")
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.BlacklistAddResponse
				res.decode(t, &got)
				assert.Equal(t, "pub-abc123", got.PeerID)
				assert.Equal(t, "203.0.113.7", got.IP)
			},
		},
		{
			"peer isn't connected to the room",
			`{"peer_id":"pub-ghost"}`,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "peer_not_found")
			},
		},
		{
			"sfu unreachable",
			`{"peer_id":"pub-abc123"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPeerIP(e.roomID, "pub-abc123", "203.0.113.7")
				sfu.setDown(true)
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")
			},
		},
		{
			"malformed body",
			`{`,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadRequest, "invalid_request")
			},
		},
		{
			"no admin token",
			`{"peer_id":"pub-abc123"}`,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnauthorized, "admin_token_required")
			},
		},
		{
			"invalid admin token",
			`{"peer_id":"pub-abc123"}`,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "not-a-jwt" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := tt.setup(t)

			res := send(t, request{method: http.MethodPost, path: "/api/blacklist", body: tt.body, token: tt.token(e)})

			tt.checkResult(t, e, res)
		})
	}
}

func TestListBlacklist(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		setup       func(t *testing.T) env
		token       func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"empty list serializes as []",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.BlacklistListResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				assert.NotNil(t, got.Blacklist)
				assert.Empty(t, got.Blacklist)
			},
		},
		{
			"blocked entries are listed",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-abc123", "203.0.113.7")
				blockIP(t, e, "pub-def456", "198.51.100.9")
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.BlacklistListResponse
				res.decode(t, &got)
				assert.ElementsMatch(t, []dto.BlacklistEntry{
					{IP: "203.0.113.7", PeerID: "pub-abc123"},
					{IP: "198.51.100.9", PeerID: "pub-def456"},
				}, got.Blacklist)
			},
		},
		{
			"list is cleared when the room closes",
			func(t *testing.T) env {
				old := openRoom(t)
				blockIP(t, old, "pub-abc123", "203.0.113.7")
				res := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + old.roomID, token: old.token})
				assertStatus(t, res, http.StatusNoContent)
				return openRoom(t)
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.BlacklistListResponse
				res.decode(t, &got)
				assert.Empty(t, got.Blacklist)
			},
		},
		{
			"no room open, so no admin token is valid",
			func(t *testing.T) env {
				old := openRoom(t)
				res := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + old.roomID, token: old.token})
				assertStatus(t, res, http.StatusNoContent)
				return old
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
		{
			"no admin token",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnauthorized, "admin_token_required")
			},
		},
		{
			"invalid admin token",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "not-a-jwt" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := tt.setup(t)

			res := send(t, request{method: http.MethodGet, path: "/api/blacklist", token: tt.token(e)})

			tt.checkResult(t, e, res)
		})
	}
}

func TestRemoveBlacklist(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		ip          string // as it appears in the URL, already encoded
		setup       func(t *testing.T) env
		token       func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"admin unblocks an ip so it can join again",
			"203.0.113.7",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-abc123", "203.0.113.7")
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusNoContent)

				join := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID, ip: "203.0.113.7"})
				assertStatus(t, join, http.StatusOK)
			},
		},
		{
			"ipv6 address is url-encoded",
			"2001%3Adb8%3A%3A1",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-abc123", "2001:db8::1")
				return e
			},
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusNoContent)
			},
		},
		{
			"ip isn't on the list",
			"203.0.113.7",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "not_blacklisted")
			},
		},
		{
			"no admin token",
			"203.0.113.7",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-abc123", "203.0.113.7")
				return e
			},
			func(e env) string { return "" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnauthorized, "admin_token_required")
			},
		},
		{
			"invalid admin token",
			"203.0.113.7",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "not-a-jwt" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := tt.setup(t)

			res := send(t, request{method: http.MethodDelete, path: "/api/blacklist/" + tt.ip, token: tt.token(e)})

			tt.checkResult(t, e, res)
		})
	}
}
