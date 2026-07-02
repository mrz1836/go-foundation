package ctxutil_test

import (
	"context"
	"testing"

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
			if got := ctxutil.RequestIDFrom(ctx); got != tt.want {
				t.Fatalf("RequestIDFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithRequestID_EmptyReturnsSameContext(t *testing.T) {
	t.Parallel()

	base := context.Background()
	got := ctxutil.WithRequestID(base, "")
	if got != base {
		t.Fatalf("WithRequestID(ctx, \"\") returned a new context; want the original unchanged")
	}
	if id := ctxutil.RequestIDFrom(got); id != "" {
		t.Fatalf("RequestIDFrom() = %q, want empty after no-op set", id)
	}
}

func TestRequestIDFrom_MissingKey(t *testing.T) {
	t.Parallel()

	if got := ctxutil.RequestIDFrom(context.Background()); got != "" {
		t.Fatalf("RequestIDFrom(Background()) = %q, want empty", got)
	}
}

func TestWithRequestID_Overwrite(t *testing.T) {
	t.Parallel()

	ctx := ctxutil.WithRequestID(context.Background(), "first")
	ctx = ctxutil.WithRequestID(ctx, "second")
	if got := ctxutil.RequestIDFrom(ctx); got != "second" {
		t.Fatalf("RequestIDFrom() after overwrite = %q, want %q", got, "second")
	}
}

// otherKey is a distinct context-key type local to the test. A value stored
// under it must be invisible to RequestIDFrom, demonstrating that the
// unexported struct key is collision-safe.
type otherKey struct{}

func TestRequestIDFrom_KeyIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), otherKey{}, "not-the-request-id")
	if got := ctxutil.RequestIDFrom(ctx); got != "" {
		t.Fatalf("RequestIDFrom() read a foreign key = %q, want empty", got)
	}
}
