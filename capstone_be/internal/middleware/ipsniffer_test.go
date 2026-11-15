package middleware_test

import (
	"capstonewebserver/internal/middleware"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetIp(t *testing.T) {
	tests := []struct {
		name  string // description of this test case
		want  *string
		want2 bool
	}{
		// TODO: Add test cases.
		// 1. context isn't populated with ip
		// 2. context has ip information attached
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got2 := middleware.GetIp(t.Context())
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("GetIp() = %v, want %v", got, tt.want)
			}
			if true {
				t.Errorf("GetIp() = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func TestIpMiddleware(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want gin.HandlerFunc
	}{
		// TODO: Add test cases.
		// 1. ipv4 user
		// 2. ipv6 user
		// 3. ip not detected ? is it possible ? remove this case if not
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := middleware.IpMiddleware()
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("IpMiddleware() = %v, want %v", got, tt.want)
			}
		})
	}
}
