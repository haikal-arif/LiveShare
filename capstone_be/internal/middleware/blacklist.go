package middleware

import (
	"context"
	"net/http"

	"capstonewebserver/internal/dto"

	"github.com/gin-gonic/gin"
)

// IPBlacklistChecker reports whether an IP is currently blacklisted. Adding
// and removing entries is a usecase responsibility (see
// internal/usecase/blacklist.go, internal/repository/blacklist.go); this
// middleware only guards requests.
type IPBlacklistChecker interface {
	IsBlocked(ctx context.Context, ip string) bool
}

// BlackListMiddleware blocks requests from a blacklisted IP (403
// blacklisted). It never mutates list itself.
func BlackListMiddleware(list IPBlacklistChecker) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ip, exists := GetIp(ctx.Request.Context())
		if !exists || ip == nil {
			ctx.Next()
			return
		}

		if list.IsBlocked(ctx.Request.Context(), *ip) {
			ctx.AbortWithStatusJSON(http.StatusForbidden, dto.ErrorResponse{
				Error: dto.ErrorDetail{Code: "blacklisted", Message: "this IP is blocked from the room"},
			})
			return
		}

		ctx.Next()
	}
}
