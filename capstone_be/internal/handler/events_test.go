package handler

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// serveEvents runs the Events handler to completion and returns what it
// wrote. Streams that don't end on their own are cut off by cancelling the
// request after wait, the way a client leaving would.
func serveEvents(t *testing.T, h WebServerHandler, wait time.Duration) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if wait > 0 {
		time.AfterFunc(wait, cancel)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: roomIdParam, Value: "ABC-123"}}

	h.Events(c)
	return w
}

func TestWebServerHandler_Events(t *testing.T) {
	template := "concert"
	stage := dto.RoomLayoutQueryResponse{RoomID: "ABC-123", Version: 3, Template: &template, Placements: []dto.Placement{}, Streamers: []dto.Streamer{}}
	closed := domain.Event{Name: domain.EventRoomClosed, Data: dto.RoomClosedEvent{RoomID: "ABC-123"}}
	updated := domain.Event{Name: domain.EventStageUpdated, Data: stage}

	tests := []struct {
		name        string
		setup       func(m *MockEventsUseCase) (cancelled *bool)
		wait        time.Duration
		checkResult func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool)
	}{
		{
			"opens with the current stage and sets the stream headers",
			func(m *MockEventsUseCase) *bool {
				cancelled := false
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(stage, make(chan domain.Event), func() { cancelled = true }, nil)
				return &cancelled
			},
			20 * time.Millisecond,
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Equal(t, http.StatusOK, res.Code)
				assert.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))
				assert.Equal(t, "no", res.Header().Get("X-Accel-Buffering"))
				assert.True(t, strings.HasPrefix(res.Body.String(), "event: stage_updated\ndata: {"), res.Body.String())
				assert.Contains(t, res.Body.String(), `"version":3`)
				assert.True(t, cancelled, "leaving must close the subscription")
			},
		},
		{
			"forwards later events",
			func(m *MockEventsUseCase) *bool {
				cancelled := false
				events := make(chan domain.Event, 1)
				events <- updated
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(stage, events, func() { cancelled = true }, nil)
				return &cancelled
			},
			20 * time.Millisecond,
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Equal(t, 2, strings.Count(res.Body.String(), "event: stage_updated\n"))
			},
		},
		{
			"room_closed is sent and then the stream ends",
			func(m *MockEventsUseCase) *bool {
				cancelled := false
				events := make(chan domain.Event, 2)
				events <- closed
				events <- updated // must never be written: the stream ended
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(stage, events, func() { cancelled = true }, nil)
				return &cancelled
			},
			0, // returns on its own; no client hang-up needed
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Contains(t, res.Body.String(), "event: room_closed\ndata: {\"room_id\":\"ABC-123\"}\n\n")
				assert.Equal(t, 1, strings.Count(res.Body.String(), "event: stage_updated\n"), "only the opening stage")
				assert.True(t, cancelled)
			},
		},
		{
			"an idle stream gets keepalive comments",
			func(m *MockEventsUseCase) *bool {
				cancelled := false
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(stage, make(chan domain.Event), func() { cancelled = true }, nil)
				return &cancelled
			},
			80 * time.Millisecond,
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Contains(t, res.Body.String(), ": keepalive\n\n")
			},
		},
		{
			"room not found is a normal json error before any event",
			func(m *MockEventsUseCase) *bool {
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(dto.RoomLayoutQueryResponse{}, nil, nil, domain.ErrRoomNotFound)
				cancelled := false
				return &cancelled
			},
			0,
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Equal(t, http.StatusNotFound, res.Code)
				assert.Contains(t, res.Header().Get("Content-Type"), "application/json")
				assert.Contains(t, res.Body.String(), "room_not_found")
				assert.NotContains(t, res.Body.String(), "event:")
			},
		},
		{
			"sfu unreachable",
			func(m *MockEventsUseCase) *bool {
				m.EXPECT().Subscribe(gomock.Any(), "ABC-123").Return(dto.RoomLayoutQueryResponse{}, nil, nil, domain.ErrSFUUnreachable)
				cancelled := false
				return &cancelled
			},
			0,
			func(t *testing.T, res *httptest.ResponseRecorder, cancelled bool) {
				assert.Equal(t, http.StatusBadGateway, res.Code)
				assert.Contains(t, res.Body.String(), "sfu_unreachable")
			},
		},
	}

	old := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = old })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockEventsUseCase(ctrl)
			cancelled := tt.setup(uc)
			h := WebServerHandler{euc: uc}

			res := serveEvents(t, h, tt.wait)
			require.NotNil(t, res)
			tt.checkResult(t, res, *cancelled)
		})
	}
}
