package middleware_test

import (
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/middleware"
	"capstonewebserver/internal/usecase"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware(t *testing.T) {
	secret := []byte("test-secret")

	validToken, err := usecase.SignAdminToken(secret, "ABC-123")
	require.NoError(t, err)

	wrongRoomToken, err := usecase.SignAdminToken(secret, "ZZZ-999")
	require.NoError(t, err)

	unsignedToken, err := usecase.SignAdminToken([]byte("not-the-server-secret"), "ABC-123")
	require.NoError(t, err)

	tests := []struct {
		name        string
		token       string
		remoteAddr  string
		checkResult func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder)
	}{
		{
			"Valid admin claim but wrong room -> rejected",
			wrongRoomToken,
			"203.0.113.7:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.True(t, c.IsAborted())
				assert.Equal(t, http.StatusForbidden, w.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, "not_admin", got.Error.Code)
			},
		},
		{
			"deformed token -> rejected",
			"not-a-jwt",
			"203.0.113.7:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.True(t, c.IsAborted())
				assert.Equal(t, http.StatusForbidden, w.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, "not_admin", got.Error.Code)
			},
		},
		{
			"good claim but isn't signed by server -> rejected",
			unsignedToken,
			"203.0.113.7:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.True(t, c.IsAborted())
				assert.Equal(t, http.StatusForbidden, w.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, "not_admin", got.Error.Code)
			},
		},
		{
			"No token given -> rejected",
			"",
			"203.0.113.7:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.True(t, c.IsAborted())
				assert.Equal(t, http.StatusUnauthorized, w.Code)
				var got dto.ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				assert.Equal(t, "admin_token_required", got.Error.Code)
			},
		},
		{
			"valid claim -> accepted",
			validToken,
			"203.0.113.7:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.False(t, c.IsAborted())
			},
		},
		{
			"valid admin claim but coming from different ip -> accepted",
			validToken,
			"198.51.100.23:54321",
			func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.False(t, c.IsAborted())
			},
		},
	}
	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodDelete, "/api/rooms/ABC-123", nil)
			c.Request.RemoteAddr = tt.remoteAddr
			if tt.token != "" {
				c.Request.Header.Set(middleware.AdminTokenHeader, tt.token)
			}

			middleware.AuthMiddleware(secret, func() string { return "ABC-123" })(c)

			tt.checkResult(t, c, w)
		})
	}
}
