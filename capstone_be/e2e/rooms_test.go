//go:build e2e

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"capstonewebserver/internal/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roomIDPattern is contracts.md's room_id: 3 + 3 characters from A-Z and
// 2-9, without the look-alikes O/0 and 1/I.
var roomIDPattern = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{3}-[A-HJ-NP-Z2-9]{3}$`)

func TestHealth(t *testing.T) {
	begin(t)

	res := send(t, request{method: http.MethodGet, path: "/api/health"})

	assertStatus(t, res, http.StatusOK)
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		origin          string
		wantAllowOrigin string
	}{
		{"allowed origin", "https://app.example.com", "https://app.example.com"},
		{"other origin", "https://evil.example.com", ""},
		{"no origin", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)

			res := send(t, request{method: http.MethodOptions, path: "/api/rooms", origin: tt.origin})

			assertStatus(t, res, http.StatusNoContent)
			assert.Equal(t, tt.wantAllowOrigin, res.header.Get("Access-Control-Allow-Origin"))
			if tt.wantAllowOrigin != "" {
				assert.Equal(t, "GET, POST, PUT, DELETE", res.header.Get("Access-Control-Allow-Methods"))
				assert.Equal(t, "Content-Type, X-Admin-Token", res.header.Get("Access-Control-Allow-Headers"))
			}
		})
	}
}

func TestCreateRoom(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		body        string
		setup       func(t *testing.T) env
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"room created",
			`{"passcode":"` + passcode + `"}`,
			nil,
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusCreated)
				var got dto.RoomCreationResponse
				res.decode(t, &got)
				assert.Regexp(t, roomIDPattern, got.RoomID)

				// admin_token is a JWT carrying admin: true and the room_id.
				parts := strings.Split(got.AdminToken, ".")
				require.Len(t, parts, 3)
				payload, err := base64.RawURLEncoding.DecodeString(parts[1])
				require.NoError(t, err)
				var claims struct {
					Admin  bool   `json:"admin"`
					RoomID string `json:"room_id"`
				}
				require.NoError(t, json.Unmarshal(payload, &claims))
				assert.True(t, claims.Admin)
				assert.Equal(t, got.RoomID, claims.RoomID)
			},
		},
		{
			"wrong passcode",
			`{"passcode":"nope"}`,
			nil,
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "invalid_passcode")
			},
		},
		{
			"missing passcode",
			`{}`,
			nil,
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "invalid_passcode")
			},
		},
		{
			"malformed body",
			`{`,
			nil,
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadRequest, "invalid_request")
			},
		},
		{
			"room already open",
			`{"passcode":"` + passcode + `"}`,
			func(t *testing.T) env { return openRoom(t) },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusConflict, "room_exists")
				var got dto.ErrorResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID, "room_id tells the frontend which room to join")
			},
		},
		{
			"wrong passcode while a room is open doesn't reveal it",
			`{"passcode":"nope"}`,
			func(t *testing.T) env { return openRoom(t) },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "invalid_passcode")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			var e env
			if tt.setup != nil {
				e = tt.setup(t)
			}

			res := send(t, request{method: http.MethodPost, path: "/api/rooms", body: tt.body, ip: creatorIP})

			tt.checkResult(t, e, res)
		})
	}
}

func TestGetRoomDetails(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		ip          string
		setup       func(t *testing.T) env
		path        func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"room with publishers",
			"198.51.100.1",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123", "pub-def456")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomQueryResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-abc123"}, {PeerID: "pub-def456"}}, got.Publishers)
				assert.Equal(t, sfuWsURL, got.SfuWsURL)
				assert.Equal(t, []dto.ICEServer{{URLs: []string{iceURL}}}, got.ICEServers)
			},
		},
		{
			"created but empty room still answers 200 with an empty publishers list",
			"198.51.100.1",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomQueryResponse
				res.decode(t, &got)
				assert.NotNil(t, got.Publishers, "must serialize as [] rather than null")
				assert.Empty(t, got.Publishers)
				assert.Equal(t, sfuWsURL, got.SfuWsURL)
			},
		},
		{
			"room never created",
			"198.51.100.1",
			nil,
			func(e env) string { return "/api/rooms/ABC-234" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"different room id than the open room",
			"198.51.100.1",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/ZZZ-999" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"sfu unreachable",
			"198.51.100.1",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setDown(true)
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")
			},
		},
		{
			"caller ip is blacklisted",
			"203.0.113.99",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-bad", "203.0.113.99")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "blacklisted")
			},
		},
		{
			"other ip isn't affected by someone else's block",
			"198.51.100.1",
			func(t *testing.T) env {
				e := openRoom(t)
				blockIP(t, e, "pub-bad", "203.0.113.99")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			var e env
			if tt.setup != nil {
				e = tt.setup(t)
			}

			res := send(t, request{method: http.MethodGet, path: tt.path(e), ip: tt.ip})

			tt.checkResult(t, e, res)
		})
	}
}

func TestGetRoomPublishers(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		setup       func(t *testing.T) env
		path        func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"room with publishers",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/publishers" },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomPublisherQueryResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-abc123"}}, got.Publishers)
			},
		},
		{
			"empty room",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID + "/publishers" },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomPublisherQueryResponse
				res.decode(t, &got)
				assert.NotNil(t, got.Publishers)
				assert.Empty(t, got.Publishers)
			},
		},
		{
			"room not found",
			nil,
			func(e env) string { return "/api/rooms/ABC-234/publishers" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"sfu unreachable",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setDown(true)
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/publishers" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			var e env
			if tt.setup != nil {
				e = tt.setup(t)
			}

			res := send(t, request{method: http.MethodGet, path: tt.path(e)})

			tt.checkResult(t, e, res)
		})
	}
}

func TestCloseRoom(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		setup       func(t *testing.T) env
		path        func(e env) string
		token       func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"admin closes the room",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusNoContent)
				assert.True(t, sfu.wasClosed(e.roomID), "SFU must be told to close the room")

				// Every /api/rooms/:room_id... call is 404 from now on.
				after := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID})
				assertError(t, after, http.StatusNotFound, "room_not_found")

				// ...and the server can host a new room.
				again := send(t, request{method: http.MethodPost, path: "/api/rooms", body: `{"passcode":"` + passcode + `"}`})
				assertStatus(t, again, http.StatusCreated)
			},
		},
		{
			"no admin token",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(e env) string { return "" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnauthorized, "admin_token_required")
			},
		},
		{
			"garbage admin token",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(e env) string { return "not-a-jwt" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
		{
			"token issued for a previous room",
			func(t *testing.T) env {
				old := openRoom(t)
				res := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + old.roomID, token: old.token})
				assertStatus(t, res, http.StatusNoContent)
				current := openRoom(t)
				return env{roomID: current.roomID, token: old.token}
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
		{
			"unknown room id with a valid token",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/ZZZ-999" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"sfu unreachable keeps the room open so the admin can retry",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setDown(true)
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")

				sfu.setDown(false)
				still := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID})
				assertStatus(t, still, http.StatusOK)

				retry := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + e.roomID, token: e.token})
				assertStatus(t, retry, http.StatusNoContent)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := tt.setup(t)

			res := send(t, request{method: http.MethodDelete, path: tt.path(e), token: tt.token(e)})

			tt.checkResult(t, e, res)
		})
	}
}

func TestReclaimRoomAdmin(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		ip          string
		setup       func(t *testing.T) env
		path        func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"same ip as the creator gets a new admin token",
			creatorIP,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID + "/admin/reclaim" },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomReclaimResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				require.NotEmpty(t, got.AdminToken)

				// The reclaimed token works on an admin endpoint.
				closed := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + e.roomID, token: got.AdminToken})
				assertStatus(t, closed, http.StatusNoContent)
			},
		},
		{
			"different ip",
			"198.51.100.23",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID + "/admin/reclaim" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "ip_mismatch")
			},
		},
		{
			"unknown room",
			creatorIP,
			nil,
			func(e env) string { return "/api/rooms/ABC-234/admin/reclaim" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			var e env
			if tt.setup != nil {
				e = tt.setup(t)
			}

			res := send(t, request{method: http.MethodPost, path: tt.path(e), ip: tt.ip})

			tt.checkResult(t, e, res)
		})
	}
}
