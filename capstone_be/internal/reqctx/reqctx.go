// Package reqctx carries the caller's IP address on a context.Context. It
// exists as its own leaf package so both internal/middleware (which sets it)
// and internal/usecase (which reads it, e.g. to record a room's creator IP)
// can depend on it without an import cycle between those two.
package reqctx

import "context"

type IPKey struct{}

// WithIP returns a copy of ctx carrying ip.
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, IPKey{}, &ip)
}

// GetIP reports the IP carried by ctx, if any.
func GetIP(ctx context.Context) (*string, bool) {
	ip, ok := ctx.Value(IPKey{}).(*string)
	return ip, ok
}
