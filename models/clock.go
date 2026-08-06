package models

import (
	"context"
	"sync"
	"time"
)

// Clock abstracts "now" so callers can be given deterministic time.
type Clock interface {
	Now(ctx context.Context) time.Time
}

// FixedClock is a Clock that always returns a constant time, regardless of the
// context or the wall clock.
type FixedClock struct {
	anchor time.Time
}

// NewFixedClock returns a FixedClock anchored at t.
func NewFixedClock(t time.Time) FixedClock {
	return FixedClock{anchor: t}
}

// Now returns the anchored time.
func (c FixedClock) Now(context.Context) time.Time {
	return c.anchor
}

// SimulatedClock is a Clock whose current time can be advanced or repositioned
// by the caller, making it suitable for deterministic tests of time-dependent
// behavior such as retries, backoff, scheduling, leases, and expiry. Where
// FixedClock reports a constant instant, a SimulatedClock lets a test move time
// forward explicitly without ever touching the wall clock, so elapsed-time
// paths become reproducible and instantaneous.
//
// A SimulatedClock is safe for concurrent use by multiple goroutines.
type SimulatedClock struct {
	mu  sync.Mutex
	now time.Time
}

// Compile-time assertion that *SimulatedClock satisfies the Clock interface.
var _ Clock = (*SimulatedClock)(nil)

// NewSimulatedClock returns a SimulatedClock whose current time starts at t.
func NewSimulatedClock(t time.Time) *SimulatedClock {
	return &SimulatedClock{now: t}
}

// Now returns the clock's current simulated time. The context is ignored; it is
// accepted only to satisfy the Clock interface.
func (c *SimulatedClock) Now(context.Context) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

// Advance moves the clock forward by d and returns the resulting current time.
// Advancing by a non-negative duration never moves the clock backwards; to
// reposition the clock to an arbitrary earlier or later instant, use Set.
func (c *SimulatedClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)

	return c.now
}

// Set replaces the clock's current time with t, which may be earlier or later
// than the time it currently reports.
func (c *SimulatedClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = t
}

// realClock is the default Clock; it returns the wall-clock time. It is
// unexported so callers can only obtain it via ClockFrom on a clock-less
// context.
type realClock struct{}

func (realClock) Now(context.Context) time.Time {
	return time.Now()
}

// clockKey is the unexported context-key type. Keeping it unexported means no
// other package can collide with or overwrite the key.
type clockKey struct{}

// WithClock returns a child context carrying clk.
func WithClock(ctx context.Context, clk Clock) context.Context {
	return context.WithValue(ctx, clockKey{}, clk)
}

// ClockFrom returns the Clock attached to ctx, or a real-time default clock
// when none is attached.
func ClockFrom(ctx context.Context) Clock {
	if clk, ok := ctx.Value(clockKey{}).(Clock); ok && clk != nil {
		return clk
	}

	return realClock{}
}
