package llm

import "context"

// sessionContextKey carries a stable conversation/session identifier from the
// caller down to the provider transport. Providers like OpenCode Go use it to
// route requests and keep prompt caching warm, so it must stay identical across
// the turns, retries, and tool calls of one conversation.
type sessionContextKey struct{}

// WithSessionID attaches a stable conversation/session identifier to ctx. A
// provider that declares a SessionHeader will send it on every request made
// with this context. An empty id is a no-op, leaving the transport's per-request
// fallback in place.
func WithSessionID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, id)
}

// SessionIDFromContext returns the conversation/session identifier attached by
// WithSessionID, or "" when the caller did not set one.
func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(sessionContextKey{}).(string)
	return id
}
