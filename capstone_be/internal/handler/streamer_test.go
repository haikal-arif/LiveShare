package handler

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestWebServerHandler_RegisterStreamer(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		streamerId  string
		body        string
		setup       func(*MockStreamerUseCase, string, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"streamer registered",
			"ABC-123",
			"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			func(m *MockStreamerUseCase, roomId, streamerId string) {
				m.EXPECT().RegisterStreamer(gomock.Any(), roomId, streamerId, dto.StreamerRegistrationRequest{
					PeerID:      "pub-abc123",
					DisplayName: "Hana A",
				}).Return(dto.StreamerRegistrationResponse{
					StreamerID:  streamerId,
					PeerID:      "pub-abc123",
					DisplayName: "Hana A",
					Live:        true,
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.StreamerRegistrationResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.True(t, got.Live)
			},
		},
		{
			"peer not live",
			"ABC-123",
			"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			func(m *MockStreamerUseCase, roomId, streamerId string) {
				m.EXPECT().RegisterStreamer(gomock.Any(), roomId, streamerId, gomock.Any()).Return(dto.StreamerRegistrationResponse{}, domain.ErrPeerNotLive)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "peer_not_live", got.Error.Code)
			},
		},
		{
			"invalid display name",
			"ABC-123",
			"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
			`{"peer_id":"pub-abc123","display_name":""}`,
			func(m *MockStreamerUseCase, roomId, streamerId string) {
				m.EXPECT().RegisterStreamer(gomock.Any(), roomId, streamerId, gomock.Any()).Return(dto.StreamerRegistrationResponse{}, domain.ErrInvalidDisplayName)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "invalid_display_name", got.Error.Code)
			},
		},
		{
			"non existing room",
			"ZZZ-999",
			"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
			`{"peer_id":"pub-abc123","display_name":"Hana A"}`,
			func(m *MockStreamerUseCase, roomId, streamerId string) {
				m.EXPECT().RegisterStreamer(gomock.Any(), roomId, streamerId, gomock.Any()).Return(dto.StreamerRegistrationResponse{}, domain.ErrRoomNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
			},
		},
		{
			"malformed body",
			"ABC-123",
			"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
			`not json`,
			func(m *MockStreamerUseCase, roomId, streamerId string) {},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, res.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockStreamerUseCase(ctrl)
			tt.setup(uc, tt.roomId, tt.streamerId)
			h := WebServerHandler{suc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{
				{Key: roomIdParam, Value: tt.roomId},
				{Key: streamerIdParam, Value: tt.streamerId},
			}
			h.RegisterStreamer(c)
			tt.checkResult(t, w)
		})
	}
}
