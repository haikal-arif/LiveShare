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

func TestWebServerHandler_AddBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		setup       func(*MockBlacklistUseCase)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"peer blocked",
			`{"peer_id":"pub-abc123"}`,
			func(m *MockBlacklistUseCase) {
				m.EXPECT().AddBlacklist(gomock.Any(), dto.BlacklistAddRequest{PeerID: "pub-abc123"}).Return(dto.BlacklistAddResponse{
					RoomID: "ABC-123",
					IP:     "203.0.113.7",
					PeerID: "pub-abc123",
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.BlacklistAddResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "203.0.113.7", got.IP)
			},
		},
		{
			"peer not found",
			`{"peer_id":"pub-missing"}`,
			func(m *MockBlacklistUseCase) {
				m.EXPECT().AddBlacklist(gomock.Any(), dto.BlacklistAddRequest{PeerID: "pub-missing"}).Return(dto.BlacklistAddResponse{}, domain.ErrPeerNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "peer_not_found", got.Error.Code)
			},
		},
		{
			"sfu unreachable",
			`{"peer_id":"pub-abc123"}`,
			func(m *MockBlacklistUseCase) {
				m.EXPECT().AddBlacklist(gomock.Any(), gomock.Any()).Return(dto.BlacklistAddResponse{}, domain.ErrSFUUnreachable)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadGateway, res.Code)
			},
		},
		{
			"malformed body",
			`not json`,
			func(m *MockBlacklistUseCase) {},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, res.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockBlacklistUseCase(ctrl)
			tt.setup(uc)
			h := WebServerHandler{buc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.AddBlacklist(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_ListBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*MockBlacklistUseCase)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"empty list",
			func(m *MockBlacklistUseCase) {
				m.EXPECT().ListBlacklist(gomock.Any()).Return(dto.BlacklistListResponse{
					RoomID:    "ABC-123",
					Blacklist: []dto.BlacklistEntry{},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				assert.JSONEq(t, `{"room_id":"ABC-123","blacklist":[]}`, res.Body.String())
			},
		},
		{
			"one entry",
			func(m *MockBlacklistUseCase) {
				m.EXPECT().ListBlacklist(gomock.Any()).Return(dto.BlacklistListResponse{
					RoomID:    "ABC-123",
					Blacklist: []dto.BlacklistEntry{{IP: "203.0.113.7", PeerID: "pub-abc123"}},
				}, nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, res.Code)
				var got dto.BlacklistListResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Len(t, got.Blacklist, 1)
			},
		},
		{
			"no room open",
			func(m *MockBlacklistUseCase) {
				m.EXPECT().ListBlacklist(gomock.Any()).Return(dto.BlacklistListResponse{}, domain.ErrRoomNotFound)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockBlacklistUseCase(ctrl)
			tt.setup(uc)
			h := WebServerHandler{buc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			h.ListBlacklist(c)
			tt.checkResult(t, w)
		})
	}
}

func TestWebServerHandler_RemoveBlacklist(t *testing.T) {
	tests := []struct {
		name        string
		ip          string
		setup       func(*MockBlacklistUseCase, string)
		checkResult func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			"ip unblocked",
			"203.0.113.7",
			func(m *MockBlacklistUseCase, ip string) {
				m.EXPECT().RemoveBlacklist(gomock.Any(), ip).Return(nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, res.Code)
				assert.Empty(t, res.Body.String())
			},
		},
		{
			"ip not on the list",
			"203.0.113.9",
			func(m *MockBlacklistUseCase, ip string) {
				m.EXPECT().RemoveBlacklist(gomock.Any(), ip).Return(domain.ErrNotBlacklisted)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, res.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(res.Body.Bytes(), &got))
				assert.Equal(t, "not_blacklisted", got.Error.Code)
			},
		},
		{
			"ipv6 address",
			"2001:db8::1",
			func(m *MockBlacklistUseCase, ip string) {
				m.EXPECT().RemoveBlacklist(gomock.Any(), ip).Return(nil)
			},
			func(t *testing.T, res *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, res.Code)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockBlacklistUseCase(ctrl)
			tt.setup(uc, tt.ip)
			h := WebServerHandler{buc: uc}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
			c.Params = gin.Params{{Key: ipParam, Value: tt.ip}}
			h.RemoveBlacklist(c)
			tt.checkResult(t, w)
		})
	}
}
