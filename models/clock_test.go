package models_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

func TestClockFrom_RealClockFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got := models.ClockFrom(ctx).Now(ctx)

	assert.WithinDuration(t, time.Now(), got, time.Second,
		"a clock-less context must yield the real clock")
}

func TestClockFrom_ReturnsAttachedClock(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	ctx := models.WithClock(context.Background(), models.NewFixedClock(anchor))

	assert.Equal(t, anchor, models.ClockFrom(ctx).Now(ctx))
}

func TestFixedClock_NowIgnoresContext(t *testing.T) {
	t.Parallel()

	anchor := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewFixedClock(anchor)

	assert.Equal(t, anchor, clk.Now(context.Background()))
	assert.Equal(t, anchor, clk.Now(context.TODO()))
}

func TestSimulatedClock_NowReturnsStart(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	assert.Equal(t, start, clk.Now(context.Background()))
}

func TestSimulatedClock_NowIgnoresContext(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	assert.Equal(t, start, clk.Now(context.Background()))
	assert.Equal(t, start, clk.Now(context.TODO()))
}

func TestSimulatedClock_AdvanceReturnsAndMovesForward(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	got := clk.Advance(90 * time.Minute)
	want := start.Add(90 * time.Minute)

	assert.Equal(t, want, got, "Advance must return the resulting time")
	assert.Equal(t, want, clk.Now(context.Background()), "Advance must persist the new time")
}

func TestSimulatedClock_AdvanceIsMonotonic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	prev := clk.Now(ctx)
	for i := 0; i < 100; i++ {
		got := clk.Advance(time.Second)
		assert.True(t, got.After(prev), "advancing by a positive duration must move strictly forward")
		assert.False(t, got.Before(prev), "advance must never move the clock backwards")
		prev = got
	}
}

func TestSimulatedClock_SetRepositions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	// Set forward.
	later := start.Add(24 * time.Hour)
	clk.Set(later)
	assert.Equal(t, later, clk.Now(ctx))

	// Set to an earlier instant behaves as documented: Set may move backwards.
	earlier := start.Add(-24 * time.Hour)
	clk.Set(earlier)
	assert.Equal(t, earlier, clk.Now(ctx))
}

func TestSimulatedClock_WithClockRoundTrip(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)
	ctx := models.WithClock(context.Background(), clk)

	// ClockFrom returns the same instance that was attached.
	got, ok := models.ClockFrom(ctx).(*models.SimulatedClock)
	require.True(t, ok, "ClockFrom must return the attached *SimulatedClock")
	assert.Same(t, clk, got, "ClockFrom must return the same instance, not a copy")

	// Advancing the original is observable through the context clock.
	clk.Advance(time.Hour)
	assert.Equal(t, start.Add(time.Hour), models.ClockFrom(ctx).Now(ctx))
}

func TestSimulatedClock_ConcurrentAdvanceAndNow(t *testing.T) {
	t.Parallel()

	const workers = 50

	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewSimulatedClock(start)

	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			clk.Advance(time.Millisecond)
		}()
		go func() {
			defer wg.Done()
			_ = clk.Now(ctx)
		}()
	}
	wg.Wait()

	assert.Equal(t, start.Add(workers*time.Millisecond), clk.Now(ctx),
		"every concurrent Advance must be applied exactly once")
}
