package ctxutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/ctxutil"
)

func TestWithRequestID_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "typical id", id: "req-123", want: "req-123"},
		{name: "uuid-shaped id", id: "018f9a2c-6b1e-7c3d-9f21-2a4b6c8d0e1f", want: "018f9a2c-6b1e-7c3d-9f21-2a4b6c8d0e1f"},
		{name: "empty id is a no-op", id: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := ctxutil.WithRequestID(context.Background(), tt.id)
			assert.Equal(t, tt.want, ctxutil.RequestIDFrom(ctx))
		})
	}
}

func TestWithRequestID_EmptyReturnsSameContext(t *testing.T) {
	t.Parallel()

	base := context.Background()
	got := ctxutil.WithRequestID(base, "")
	assert.Equal(t, base, got, "WithRequestID(ctx, \"\") must return the original context unchanged")
	assert.Empty(t, ctxutil.RequestIDFrom(got))
}

func TestRequestIDFrom_MissingKey(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ctxutil.RequestIDFrom(context.Background()))
}

func TestWithRequestID_Overwrite(t *testing.T) {
	t.Parallel()

	ctx := ctxutil.WithRequestID(context.Background(), "first")
	ctx = ctxutil.WithRequestID(ctx, "second")
	assert.Equal(t, "second", ctxutil.RequestIDFrom(ctx))
}

// otherKey is a distinct context-key type local to the test. A value stored
// under it must be invisible to RequestIDFrom, demonstrating that the
// unexported struct key is collision-safe.
type otherKey struct{}

func TestRequestIDFrom_KeyIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), otherKey{}, "not-the-request-id")
	assert.Empty(t, ctxutil.RequestIDFrom(ctx))
}
