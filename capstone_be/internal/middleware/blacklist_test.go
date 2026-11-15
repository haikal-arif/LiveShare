package middleware

import (
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/reqctx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlackListMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		ip          string
		setup       func(list *repository.BlacklistStore)
		checkResult func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder)
	}{
		{
			name: "non-blacklisted request pass through",
			ip:   "203.0.113.7",
			checkResult: func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.False(t, c.IsAborted())
			},
		},
		{
			name: "blacklisted ip got blocked",
			ip:   "203.0.113.7",
			setup: func(list *repository.BlacklistStore) {
				_, err := list.Add(t.Context(), "203.0.113.7", "pub-abc123")
				require.NoError(t, err)
			},
			checkResult: func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.True(t, c.IsAborted())
				assert.Equal(t, http.StatusForbidden, w.Code)
			},
		},
		{
			name: "ip missing from context passes through",
			ip:   "",
			checkResult: func(t *testing.T, c *gin.Context, w *httptest.ResponseRecorder) {
				assert.False(t, c.IsAborted())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := repository.NewBlacklistStore()
			if tt.setup != nil {
				tt.setup(list)
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.ip != "" {
				c.Request = c.Request.WithContext(reqctx.WithIP(c.Request.Context(), tt.ip))
			}

			BlackListMiddleware(list)(c)

			tt.checkResult(t, c, w)
		})
	}
}
