package middleware

import (
	"net/http"

	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/usecase"

	"github.com/gin-gonic/gin"
)

var (
	AdminTokenHeader = "X-Admin-Token"
)

// RoomIDProvider returns the id of the currently open room, or "" if no room
// is open.
type RoomIDProvider func() string

// AuthMiddleware checks the X-Admin-Token header against secret and the
// room the token was issued for, per contracts.md's "Admin endpoints":
// missing header -> 401 admin_token_required; invalid token, or a token
// issued for a different room -> 403 not_admin.
func AuthMiddleware(secret []byte, currentRoomID RoomIDProvider) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader(AdminTokenHeader)
		if token == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{
				Error: dto.ErrorDetail{Code: "admin_token_required", Message: "X-Admin-Token header is required"},
			})
			return
		}

		claims, err := usecase.VerifyAdminToken(token, secret)
		if err != nil || !claims.Admin || claims.RoomID != currentRoomID() {
			ctx.AbortWithStatusJSON(http.StatusForbidden, dto.ErrorResponse{
				Error: dto.ErrorDetail{Code: "not_admin", Message: "admin token does not match this room"},
			})
			return
		}

		ctx.Next()
	}
}
