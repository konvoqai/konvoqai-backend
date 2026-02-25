package utils

import "context"

type requestIDContextKey struct{}

// WithRequestID stores request ID in context for downstream handlers.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// RequestID returns request ID from context when present.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDContextKey{}).(string)
	return v
}
