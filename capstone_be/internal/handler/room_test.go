package handler

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/middleware"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestWebServerHandler_CreateRoom(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		body        string
		setup       func(*MockRoomUseCase)
		checkResult func(t *testing.T, body *httptest.ResponseRecorder)
	}{
		{
			"room created",
			`{"passcode":"the-passcode-from-env"}`,
			func(m *MockRoomUseCase) {
				m.EXPECT().CreateRoom(gomock.Any(), "the-passcode-from-env").Return(dto.RoomCreationResponse{
					RoomID:     "ABC-123",
					AdminToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl",
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusCreated, res.Code)
				var got dto.RoomCreationResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "ABC-123", got.RoomID)
				assert.Equal(t, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl", got.AdminToken)
			},
		},
		{
			"wrong passcode",
			`{"passcode":"nope"}`,
			func(m *MockRoomUseCase) {
				m.EXPECT().CreateRoom(gomock.Any(), "nope").Return(dto.RoomCreationResponse{}, domain.ErrInvalidPasscode)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "invalid_passcode", got.Error.Code)
			},
		},
		{
			"room already created",
			`{"passcode":"the-passcode-from-env"}`,
			func(m *MockRoomUseCase) {
				m.EXPECT().CreateRoom(gomock.Any(), "the-passcode-from-env").Return(dto.RoomCreationResponse{}, &domain.RoomExistsError{RoomID: "ABC-123"})
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusConflict, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "room_exists", got.Error.Code)
				assert.Equal(t, "ABC-123", got.RoomID)
			},
		},
		{
			"unexpected use case error",
			`{"passcode":"the-passcode-from-env"}`,
			func(m *MockRoomUseCase) {
				m.EXPECT().CreateRoom(gomock.Any(), "the-passcode-from-env").Return(dto.RoomCreationResponse{}, errors.New("db down"))
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusInternalServerError, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "internal_error", got.Error.Code)
			},
		},
		{
			"malformed body",
			`not json`,
			func(m *MockRoomUseCase) {},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, res.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockRoomUseCase(ctrl)
			tt.setup(uc)
			h := WebServerHandler{ruc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.CreateRoom(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_GetRoomDetails(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		roomId      string
		setup       func(*MockRoomUseCase, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"ExistingRoom",
			"ABC-123",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomDetail(gomock.Any(), roomId).Return(dto.RoomQueryResponse{
					RoomID:     roomId,
					Publishers: []dto.Publisher{{PeerID: "pub-abc123"}, {PeerID: "pub-def456"}},
					SfuWsURL:   "wss://sfu.example.com/ws",
					ICEServers: []dto.ICEServer{{URLs: []string{"stun:stun.example.com:3478"}}},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.RoomQueryResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "ABC-123", got.RoomID)
				assert.Len(t, got.Publishers, 2)
				assert.Equal(t, "wss://sfu.example.com/ws", got.SfuWsURL)
				assert.Equal(t, []string{"stun:stun.example.com:3478"}, got.ICEServers[0].URLs)
			},
		},
		{
			"Non-ExistingRoom",
			"ZZZ-999",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomDetail(gomock.Any(), roomId).Return(dto.RoomQueryResponse{}, domain.ErrRoomNotFound)
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
			uc := NewMockRoomUseCase(ctrl)
			tt.setup(uc, tt.roomId)
			h := WebServerHandler{ruc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Params = gin.Params{{Key: roomIdParam, Value: tt.roomId}}
			h.GetRoomDetails(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_GetRoomPublishers(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		roomId      string
		setup       func(*MockRoomUseCase, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"Existing empty room",
			"ABC-123",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomPublishers(gomock.Any(), roomId).Return(dto.RoomPublisherQueryResponse{
					RoomID:     roomId,
					Publishers: []dto.Publisher{},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				// The contract requires an empty array, not null.
				assert.JSONEq(t, `{"room_id":"ABC-123","publishers":[]}`, res.Body.String())
			},
		},
		{
			"Room with one publisher",
			"ABC-123",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomPublishers(gomock.Any(), roomId).Return(dto.RoomPublisherQueryResponse{
					RoomID:     roomId,
					Publishers: []dto.Publisher{{PeerID: "pub-abc123"}},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.RoomPublisherQueryResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-abc123"}}, got.Publishers)
			},
		},
		{
			"Room with 5 publisher",
			"ABC-123",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomPublishers(gomock.Any(), roomId).Return(dto.RoomPublisherQueryResponse{
					RoomID: roomId,
					Publishers: []dto.Publisher{
						{PeerID: "p1"}, {PeerID: "p2"}, {PeerID: "p3"}, {PeerID: "p4"}, {PeerID: "p5"},
					},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.RoomPublisherQueryResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Len(t, got.Publishers, 5)
			},
		},
		{
			"Non-existing/non created room",
			"ZZZ-999",
			func(m *MockRoomUseCase, roomId string) {
				m.EXPECT().RoomPublishers(gomock.Any(), roomId).Return(dto.RoomPublisherQueryResponse{}, domain.ErrRoomNotFound)
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
			uc := NewMockRoomUseCase(ctrl)
			tt.setup(uc, tt.roomId)
			h := WebServerHandler{ruc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Params = gin.Params{{Key: roomIdParam, Value: tt.roomId}}
			h.GetRoomPublishers(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_CloseRoom(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		setup       func(*MockRoomUseCase)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"Request doesn't come from room owner",
			func(m *MockRoomUseCase) {
				m.EXPECT().CloseRoom(gomock.Any(), "ABC-123").Return(domain.ErrNotAdmin)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "not_admin", got.Error.Code)
			},
		},
		{
			"Closing an already closed room",
			func(m *MockRoomUseCase) {
				m.EXPECT().CloseRoom(gomock.Any(), "ABC-123").Return(domain.ErrRoomNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "room_not_found", got.Error.Code)
			},
		},
		{
			"Closing non existing room",
			func(m *MockRoomUseCase) {
				m.EXPECT().CloseRoom(gomock.Any(), "ABC-123").Return(domain.ErrRoomNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
			},
		},
		{
			"SFU unreachable",
			func(m *MockRoomUseCase) {
				m.EXPECT().CloseRoom(gomock.Any(), "ABC-123").Return(domain.ErrSFUUnreachable)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadGateway, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "sfu_unreachable", got.Error.Code)
			},
		},
		{
			"success closing the room",
			func(m *MockRoomUseCase) {
				m.EXPECT().CloseRoom(gomock.Any(), "ABC-123").Return(nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, res.Code)
				assert.Empty(t, res.Body.String())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockRoomUseCase(ctrl)
			tt.setup(uc)
			h := WebServerHandler{ruc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
			c.Params = gin.Params{{Key: roomIdParam, Value: "ABC-123"}}
			h.CloseRoom(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_ReclaimRoomAdmin(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		remoteAddr  string
		setup       func(*MockRoomUseCase)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"request doesn't come from the same ip as recorded",
			"ABC-123",
			"203.0.113.9:54321",
			func(m *MockRoomUseCase) {
				m.EXPECT().ReclaimRoom(gomock.Any(), "ABC-123", "203.0.113.9").Return(dto.RoomReclaimResponse{}, domain.ErrIPMismatch)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "ip_mismatch", got.Error.Code)
			},
		},
		{
			"reclaim proceeds successfully",
			"ABC-123",
			"203.0.113.7:54321",
			func(m *MockRoomUseCase) {
				m.EXPECT().ReclaimRoom(gomock.Any(), "ABC-123", "203.0.113.7").Return(dto.RoomReclaimResponse{
					RoomID:     "ABC-123",
					AdminToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl",
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.RoomReclaimResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "ABC-123", got.RoomID)
				assert.Equal(t, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl", got.AdminToken)
			},
		},
		{
			"non existing room",
			"ZZZ-999",
			"203.0.113.7:54321",
			func(m *MockRoomUseCase) {
				m.EXPECT().ReclaimRoom(gomock.Any(), "ZZZ-999", "203.0.113.7").Return(dto.RoomReclaimResponse{}, domain.ErrRoomNotFound)
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
			uc := NewMockRoomUseCase(ctrl)
			tt.setup(uc)
			h := WebServerHandler{ruc: uc}
			w := httptest.NewRecorder()

			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.RemoteAddr = tt.remoteAddr
			c.Params = gin.Params{{Key: roomIdParam, Value: tt.roomId}}
			middleware.IpMiddleware()(c)

			h.ReclaimRoomAdmin(c)
			tt.checkResult(t, w)
		})
	}
}
