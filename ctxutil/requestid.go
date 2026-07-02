// Package ctxutil provides small, domain-agnostic helpers for threading values
// through context.Context. It currently carries an HTTP request id so a single
// id can be propagated across handler, service, and worker boundaries without
// callers threading it explicitly.
package ctxutil

import "context"

// requestIDKey types the context key used to thread an HTTP request id through
// a call chain. Using an unexported zero-size struct as the key keeps it
// collision-safe: no other package can accidentally read or overwrite the value.
type requestIDKey struct{}

// WithRequestID returns ctx tagged with id. An empty id is a no-op so callers
// can pass the value through without branching.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom returns the request id stamped on ctx, or "" when none is
// present. Services and workers use this to read the id without taking a
// dependency on the HTTP layer that originally set it.
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}
