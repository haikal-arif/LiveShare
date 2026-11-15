//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"capstonewebserver/internal/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const streamerA = "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11"

func TestRegisterStreamer(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		body        string
		setup       func(t *testing.T) env
		path        func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"live publisher registers",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.Streamer
				res.decode(t, &got)
				assert.Equal(t, dto.Streamer{StreamerID: streamerA, PeerID: "pub-abc123", DisplayName: "Hana A", Live: true}, got)
			},
		},
		{
			"display name is trimmed",
			`{"peer_id":"pub-abc123","display_name":"  Hana A  "}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.Streamer
				res.decode(t, &got)
				assert.Equal(t, "Hana A", got.DisplayName)
			},
		},
		{
			"display name of exactly 40 characters",
			`{"peer_id":"pub-abc123","display_name":"` + strings.Repeat("a", 40) + `"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
			},
		},
		{
			"re-registering after a reconnect updates the peer_id (idempotent)",
			`{"peer_id":"pub-new999","display_name":"Hana B"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123", "pub-new999")
				registerStreamer(t, e, streamerA, "pub-abc123", "Hana A")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.Streamer
				res.decode(t, &got)
				assert.Equal(t, "pub-new999", got.PeerID)
				assert.Equal(t, "Hana B", got.DisplayName)

				stage := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID + "/stage"})
				var gotStage dto.RoomLayoutQueryResponse
				stage.decode(t, &gotStage)
				assert.Len(t, gotStage.Streamers, 1, "same streamer_id must not be registered twice")
			},
		},
		{
			"blank display name",
			`{"peer_id":"pub-abc123","display_name":"   "}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_display_name")
			},
		},
		{
			"display name over 40 characters",
			`{"peer_id":"pub-abc123","display_name":"` + strings.Repeat("a", 41) + `"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_display_name")
			},
		},
		{
			"peer_id isn't a live publisher",
			`{"peer_id":"pub-ghost","display_name":"Hana A"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "peer_not_live")
			},
		},
		{
			"malformed body",
			`{`,
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadRequest, "invalid_request")
			},
		},
		{
			"room not found",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			nil,
			func(e env) string { return "/api/rooms/ABC-234/streamers/" + streamerA },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"sfu unreachable",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setDown(true)
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/streamers/" + streamerA },
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

			res := send(t, request{method: http.MethodPut, path: tt.path(e), body: tt.body})

			tt.checkResult(t, e, res)
		})
	}
}

func TestGetStage(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		setup       func(t *testing.T) env
		path        func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"no stage set yet",
			func(t *testing.T) env { return openRoom(t) },
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomLayoutQueryResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				assert.Zero(t, got.Version)
				assert.Nil(t, got.Template)
				assert.NotNil(t, got.Placements, "must serialize as [] rather than null")
				assert.Empty(t, got.Placements)
				assert.Empty(t, got.Streamers)
			},
		},
		{
			"registered streamers carry their liveness",
			func(t *testing.T) env {
				e := openRoom(t)
				sfu.setPublishers(e.roomID, "pub-abc123", "pub-def456")
				registerStreamer(t, e, "streamer-a", "pub-abc123", "Hana A")
				registerStreamer(t, e, "streamer-b", "pub-def456", "User C")
				// streamer-b leaves; it stays registered, just not live.
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomLayoutQueryResponse
				res.decode(t, &got)
				assert.Equal(t, int64(2), got.Version, "version goes up by 1 on every change to the streamers")
				assert.Equal(t, []dto.Streamer{
					{StreamerID: "streamer-a", PeerID: "pub-abc123", DisplayName: "Hana A", Live: true},
					{StreamerID: "streamer-b", PeerID: "pub-def456", DisplayName: "User C", Live: false},
				}, got.Streamers)
			},
		},
		{
			"room not found",
			nil,
			func(e env) string { return "/api/rooms/ABC-234/stage" },
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
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
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

func TestPutStage(t *testing.T) {
	// twoStreamers opens a room with two live, registered streamers.
	twoStreamers := func(t *testing.T) env {
		e := openRoom(t)
		sfu.setPublishers(e.roomID, "pub-abc123", "pub-def456")
		registerStreamer(t, e, "streamer-a", "pub-abc123", "Hana A")
		registerStreamer(t, e, "streamer-b", "pub-def456", "User C")
		return e
	}

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		body        string
		setup       func(t *testing.T) env
		path        func(e env) string
		token       func(e env) string
		checkResult func(t *testing.T, e env, res response)
	}{
		{
			"admin saves a layout",
			`{"template":"concert","placements":[{"streamer_id":"streamer-a","x":0.3,"y":0.23},{"streamer_id":"streamer-b","x":0.75,"y":0.45}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomLayoutQueryResponse
				res.decode(t, &got)
				assert.Equal(t, e.roomID, got.RoomID)
				assert.Equal(t, int64(3), got.Version, "2 streamer registrations + 1 layout save")
				require.NotNil(t, got.Template)
				assert.Equal(t, "concert", *got.Template)
				assert.Equal(t, []dto.Placement{
					{StreamerID: "streamer-a", X: 0.3, Y: 0.23},
					{StreamerID: "streamer-b", X: 0.75, Y: 0.45},
				}, got.Placements)
				assert.Len(t, got.Streamers, 2)

				// GET returns exactly what the PUT stored.
				stage := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID + "/stage"})
				var gotGet dto.RoomLayoutQueryResponse
				stage.decode(t, &gotGet)
				assert.Equal(t, got, gotGet)
			},
		},
		{
			"every known template is accepted",
			`{"template":"blank","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
			},
		},
		{
			"null template with empty placements removes the stage",
			`{"template":null,"placements":[]}`,
			func(t *testing.T) env {
				e := twoStreamers(t)
				res := send(t, request{
					method: http.MethodPut, path: "/api/rooms/" + e.roomID + "/stage", token: e.token,
					body: `{"template":"theater","placements":[{"streamer_id":"streamer-a","x":0.5,"y":0.5}]}`,
				})
				assertStatus(t, res, http.StatusOK)
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomLayoutQueryResponse
				res.decode(t, &got)
				assert.Nil(t, got.Template)
				assert.NotNil(t, got.Placements)
				assert.Empty(t, got.Placements)
				assert.Len(t, got.Streamers, 2, "streamers stay registered")
			},
		},
		{
			"placement for a streamer who isn't live is kept",
			`{"template":"concert","placements":[{"streamer_id":"streamer-b","x":0.1,"y":0.9}]}`,
			func(t *testing.T) env {
				e := twoStreamers(t)
				sfu.setPublishers(e.roomID, "pub-abc123")
				return e
			},
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
				var got dto.RoomLayoutQueryResponse
				res.decode(t, &got)
				assert.Len(t, got.Placements, 1)
				assert.False(t, got.Streamers[1].Live)
			},
		},
		{
			"positions on the edges are valid",
			`{"template":"concert","placements":[{"streamer_id":"streamer-a","x":0,"y":1}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertStatus(t, res, http.StatusOK)
			},
		},
		{
			"unknown template",
			`{"template":"stadium","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_template")
			},
		},
		{
			"x outside 0.0-1.0",
			`{"template":"concert","placements":[{"streamer_id":"streamer-a","x":1.01,"y":0.5}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_position")
			},
		},
		{
			"y outside 0.0-1.0",
			`{"template":"concert","placements":[{"streamer_id":"streamer-a","x":0.5,"y":-0.1}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_position")
			},
		},
		{
			"streamer that isn't registered",
			`{"template":"concert","placements":[{"streamer_id":"streamer-nobody","x":0.5,"y":0.5}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "unknown_streamer")
			},
		},
		{
			"same streamer placed twice",
			`{"template":"concert","placements":[{"streamer_id":"streamer-a","x":0.1,"y":0.1},{"streamer_id":"streamer-a","x":0.2,"y":0.2}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "duplicate_streamer")
			},
		},
		{
			"placements with a null template",
			`{"template":null,"placements":[{"streamer_id":"streamer-a","x":0.1,"y":0.1}]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "placements_without_template")
			},
		},
		{
			"a rejected save leaves the stage untouched",
			`{"template":"stadium","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnprocessableEntity, "invalid_template")

				stage := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID + "/stage"})
				var got dto.RoomLayoutQueryResponse
				stage.decode(t, &got)
				assert.Equal(t, int64(2), got.Version)
				assert.Nil(t, got.Template)
			},
		},
		{
			"malformed body",
			`{`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusBadRequest, "invalid_request")
			},
		},
		{
			"no admin token",
			`{"template":"concert","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return "" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusUnauthorized, "admin_token_required")
			},
		},
		{
			"invalid admin token",
			`{"template":"concert","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/" + e.roomID + "/stage" },
			func(e env) string { return "not-a-jwt" },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusForbidden, "not_admin")
			},
		},
		{
			"room not found",
			`{"template":"concert","placements":[]}`,
			twoStreamers,
			func(e env) string { return "/api/rooms/ZZZ-999/stage" },
			func(e env) string { return e.token },
			func(t *testing.T, e env, res response) {
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := tt.setup(t)

			res := send(t, request{method: http.MethodPut, path: tt.path(e), body: tt.body, token: tt.token(e)})

			tt.checkResult(t, e, res)
		})
	}
}
