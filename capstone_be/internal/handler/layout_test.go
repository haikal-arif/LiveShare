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

func TestWebServerHandler_GetStage(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		setup       func(*MockLayoutUseCase, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"stage set",
			"ABC-123",
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().GetStage(gomock.Any(), roomId).Return(dto.RoomLayoutQueryResponse{
					RoomID:   roomId,
					Version:  7,
					Template: func() *string { v := "concert"; return &v }(),
					Placements: []dto.Placement{
						{StreamerID: "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11", X: 0.3, Y: 0.23},
					},
					Streamers: []dto.Streamer{
						{StreamerID: "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11", PeerID: "pub-abc123", DisplayName: "Hana A", Live: true},
					},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.RoomLayoutQueryResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, int64(7), got.Version)
				assert.Equal(t, "concert", *got.Template)
				assert.Len(t, got.Placements, 1)
			},
		},
		{
			"no stage set yet",
			"ABC-123",
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().GetStage(gomock.Any(), roomId).Return(dto.RoomLayoutQueryResponse{
					RoomID:     roomId,
					Version:    0,
					Template:   nil,
					Placements: []dto.Placement{},
					Streamers:  []dto.Streamer{},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				assert.JSONEq(t, `{"room_id":"ABC-123","version":0,"template":null,"placements":[],"streamers":[]}`, res.Body.String())
			},
		},
		{
			"non existing room",
			"ZZZ-999",
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().GetStage(gomock.Any(), roomId).Return(dto.RoomLayoutQueryResponse{}, domain.ErrRoomNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "room_not_found", got.Error.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockLayoutUseCase(ctrl)
			tt.setup(uc, tt.roomId)
			h := WebServerHandler{luc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Params = gin.Params{{Key: roomIdParam, Value: tt.roomId}}
			h.GetStage(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_PutStage(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		body        string
		setup       func(*MockLayoutUseCase, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"layout saved",
			"ABC-123",
			`{"template":"concert","placements":[{"streamer_id":"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11","x":0.3,"y":0.23}]}`,
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().PutStage(gomock.Any(), roomId, dto.RoomLayoutEditRequest{
					Template: func() *string { v := "concert"; return &v }(),
					Placements: []dto.Placement{
						{StreamerID: "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11", X: 0.3, Y: 0.23},
					},
				}).Return(dto.RoomLayoutQueryResponse{RoomID: roomId, Version: 1, Template: func() *string { v := "concert"; return &v }()}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
			},
		},
		{
			"invalid template",
			"ABC-123",
			`{"template":"castle","placements":[]}`,
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().PutStage(gomock.Any(), roomId, dto.RoomLayoutEditRequest{
					Template:   func() *string { v := "castle"; return &v }(),
					Placements: []dto.Placement{},
				}).Return(dto.RoomLayoutQueryResponse{}, domain.ErrInvalidTemplate)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "invalid_template", got.Error.Code)
			},
		},
		{
			"placements without template",
			"ABC-123",
			`{"template":null,"placements":[{"streamer_id":"5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11","x":0.3,"y":0.23}]}`,
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().PutStage(gomock.Any(), roomId, gomock.Any()).Return(dto.RoomLayoutQueryResponse{}, domain.ErrPlacementsWithoutTemplate)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "placements_without_template", got.Error.Code)
			},
		},
		{
			"malformed body",
			"ABC-123",
			`not json`,
			func(m *MockLayoutUseCase, roomId string) {},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, res.Code)
			},
		},
		{
			"not admin",
			"ABC-123",
			`{"template":null,"placements":[]}`,
			func(m *MockLayoutUseCase, roomId string) {
				m.EXPECT().PutStage(gomock.Any(), roomId, gomock.Any()).Return(dto.RoomLayoutQueryResponse{}, domain.ErrNotAdmin)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "not_admin", got.Error.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockLayoutUseCase(ctrl)
			tt.setup(uc, tt.roomId)
			h := WebServerHandler{luc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: roomIdParam, Value: tt.roomId}}
			h.PutStage(c)
			tt.checkResult(t, w)
		})
	}
}
