//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"capstonewebserver/internal/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseEvent is one parsed event from the stream. Comment lines (": ...") are
// reported as an event named ":comment".
type sseEvent struct {
	name string
	data string
}

// eventStream is an open GET /api/rooms/:room_id/events connection.
type eventStream struct {
	res    *http.Response
	events chan sseEvent
	cancel context.CancelFunc
}

// openStream connects to the room's events stream and starts parsing it.
// It's closed when the test ends.
func openStream(t *testing.T, roomID string) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/rooms/"+roomID+"/events", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	s := &eventStream{res: res, events: make(chan sseEvent, 64), cancel: cancel}
	t.Cleanup(func() {
		cancel()
		res.Body.Close()
	})
	go func() {
		defer close(s.events)
		var ev sseEvent
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if ev.name != "" {
					s.events <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, ":"):
				s.events <- sseEvent{name: ":comment", data: line}
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return s
}

// next waits for the next event, failing the test if none arrives.
func (s *eventStream) next(t *testing.T) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-s.events:
		require.True(t, ok, "stream ended while waiting for an event")
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
		return sseEvent{}
	}
}

// nextStage waits for a stage_updated and decodes it.
func (s *eventStream) nextStage(t *testing.T) dto.RoomLayoutQueryResponse {
	t.Helper()
	ev := s.next(t)
	require.Equal(t, "stage_updated", ev.name, "data: %s", ev.data)
	var stage dto.RoomLayoutQueryResponse
	require.NoError(t, json.Unmarshal([]byte(ev.data), &stage))
	return stage
}

// waitClosed waits for the server to end the stream.
func (s *eventStream) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case ev, ok := <-s.events:
		require.False(t, ok, "expected the stream to end, got %q", ev.name)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stream to end")
	}
}

func TestEvents(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, e env)
	}{
		{
			"opens with the current stage and the stream headers",
			func(t *testing.T, e env) {
				s := openStream(t, e.roomID)
				assert.Equal(t, http.StatusOK, s.res.StatusCode)
				assert.Equal(t, "text/event-stream", s.res.Header.Get("Content-Type"))
				assert.Equal(t, "no", s.res.Header.Get("X-Accel-Buffering"))

				stage := s.nextStage(t)
				assert.Equal(t, e.roomID, stage.RoomID)
				assert.Equal(t, int64(0), stage.Version)
				assert.Nil(t, stage.Template)
			},
		},
		{
			"a stage PUT is pushed to every open stream",
			func(t *testing.T, e env) {
				a, b := openStream(t, e.roomID), openStream(t, e.roomID)
				a.nextStage(t)
				b.nextStage(t)

				res := send(t, request{method: http.MethodPut, path: "/api/rooms/" + e.roomID + "/stage", token: e.token, body: `{"template":"concert","placements":[]}`})
				assertStatus(t, res, http.StatusOK)

				for _, s := range []*eventStream{a, b} {
					stage := s.nextStage(t)
					require.NotNil(t, stage.Template)
					assert.Equal(t, "concert", *stage.Template)
					assert.Equal(t, int64(1), stage.Version)
				}
			},
		},
		{
			"registering a streamer is pushed",
			func(t *testing.T, e env) {
				s := openStream(t, e.roomID)
				s.nextStage(t)
				sfu.setPublishers(e.roomID, "pub-abc123")

				registerStreamer(t, e, "streamer-1", "pub-abc123", "Hana A")

				stage := s.nextStage(t)
				assert.Equal(t, []dto.Streamer{{StreamerID: "streamer-1", PeerID: "pub-abc123", DisplayName: "Hana A", Live: true}}, stage.Streamers)
			},
		},
		{
			"a streamer going offline is noticed and pushed",
			func(t *testing.T, e env) {
				sfu.setPublishers(e.roomID, "pub-abc123")
				registerStreamer(t, e, "streamer-1", "pub-abc123", "Hana A")
				s := openStream(t, e.roomID)
				first := s.nextStage(t)
				require.True(t, first.Streamers[0].Live)

				sfu.setPublishers(e.roomID) // the publisher leaves the SFU

				// The watcher polls every 2s and may first resend the unchanged
				// stage it has no baseline for; wait for the change itself.
				deadline := time.After(10 * time.Second)
				for {
					select {
					case ev := <-s.events:
						var stage dto.RoomLayoutQueryResponse
						require.NoError(t, json.Unmarshal([]byte(ev.data), &stage))
						if !stage.Streamers[0].Live {
							return
						}
					case <-deadline:
						t.Fatal("never saw the streamer go offline")
					}
				}
			},
		},
		{
			"closing the room sends room_closed and ends the stream",
			func(t *testing.T, e env) {
				s := openStream(t, e.roomID)
				s.nextStage(t)

				res := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + e.roomID, token: e.token})
				assertStatus(t, res, http.StatusNoContent)

				ev := s.next(t)
				assert.Equal(t, "room_closed", ev.name)
				assert.JSONEq(t, `{"room_id":"`+e.roomID+`"}`, ev.data)
				s.waitClosed(t)
			},
		},
		{
			"a room that doesn't exist is a json 404",
			func(t *testing.T, e env) {
				res := send(t, request{method: http.MethodGet, path: "/api/rooms/ZZZ-999/events"})
				assertError(t, res, http.StatusNotFound, "room_not_found")
			},
		},
		{
			"sfu unreachable is a json 502 before any event",
			func(t *testing.T, e env) {
				sfu.setDown(true)
				res := send(t, request{method: http.MethodGet, path: "/api/rooms/" + e.roomID + "/events"})
				assertError(t, res, http.StatusBadGateway, "sfu_unreachable")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin(t)
			e := openRoom(t)
			tt.run(t, e)
		})
	}
}
