package testutil

import (
	"sync"
	"time"
)

// FakeClock is a concurrency-safe, manually controlled clock for tests. Pass its
// Now method into a component's clock seam (for example cache.WithNowFunc) and
// drive time deterministically with Advance and Set instead of depending on the
// wall clock.
type FakeClock struct {
	mu      sync.RWMutex
	current time.Time
}

// NewFakeClock returns a FakeClock started at the given time.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{current: start}
}

// Now returns the clock's current time.
func (c *FakeClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.current
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.current = c.current.Add(d)
}

// Set moves the clock to the absolute time t.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.current = t
}
