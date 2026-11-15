package middleware

import (
	"context"

	"capstonewebserver/internal/reqctx"

	"github.com/gin-gonic/gin"
)

func IpMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ip := ctx.ClientIP()
		ctx.Request = ctx.Request.WithContext(reqctx.WithIP(ctx.Request.Context(), ip))
		ctx.Next()
	}
}

func GetIp(ctx context.Context) (*string, bool) {
	return reqctx.GetIP(ctx)
}
