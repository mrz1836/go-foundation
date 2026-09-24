package testutil_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/testutil"
)

func TestFakeClock(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := testutil.NewFakeClock(start)

	assert.Equal(t, start, clock.Now(), "Now returns the start time")

	clock.Advance(90 * time.Minute)
	assert.Equal(t, start.Add(90*time.Minute), clock.Now(), "Advance moves the clock forward")

	reset := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(reset)
	assert.Equal(t, reset, clock.Now(), "Set moves the clock to an absolute time")
}
