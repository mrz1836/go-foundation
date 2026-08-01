package backoff_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/backoff"
)

// TestExponentialEdges tables Exponential over attempt edge cases and asserts it
// normalizes attempt < 1 to 1, doubles once per attempt, caps at maxDelay, and
// never overflows on a large attempt.
func TestExponentialEdges(t *testing.T) {
	t.Parallel()

	const base = time.Second
	const maxDelay = time.Minute

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: -5, want: base}, // attempt < 1 normalizes to 1
		{attempt: 0, want: base},
		{attempt: 1, want: base},
		{attempt: 2, want: 2 * base},
		{attempt: 3, want: 4 * base},
		{attempt: 7, want: maxDelay},         // 64s would exceed 60s — capped
		{attempt: 1_000_000, want: maxDelay}, // large attempt returns the cap, no overflow
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, backoff.Exponential(base, maxDelay, tc.attempt),
			"Exponential(base, max, %d)", tc.attempt)
	}
}

// TestExponentialMonotonicAndBounded sweeps a wide attempt range and asserts the
// ladder is always positive, capped at maxDelay, and monotonic non-decreasing —
// the contract callers rely on when they layer jitter or a give-up rule on top.
func TestExponentialMonotonicAndBounded(t *testing.T) {
	t.Parallel()

	const base = time.Second
	const maxDelay = time.Minute

	var prev time.Duration
	for attempt := 0; attempt <= 64; attempt++ {
		d := backoff.Exponential(base, maxDelay, attempt)
		assert.Positivef(t, d, "delay is never zero or negative (attempt %d)", attempt)
		assert.LessOrEqualf(t, d, maxDelay, "delay is capped at max (attempt %d)", attempt)
		assert.GreaterOrEqualf(t, d, prev, "delay is monotonic non-decreasing (attempt %d)", attempt)
		prev = d
	}
}

// TestExponentialGrowsThenCaps pins the common one-second-to-one-minute schedule:
// it starts at the base, doubles, and holds flat at the cap.
func TestExponentialGrowsThenCaps(t *testing.T) {
	t.Parallel()

	assert.Equal(t, time.Second, backoff.Exponential(time.Second, time.Minute, 0), "attempt below 1 is treated as 1")
	assert.Equal(t, time.Second, backoff.Exponential(time.Second, time.Minute, 1))
	assert.Equal(t, 2*time.Second, backoff.Exponential(time.Second, time.Minute, 2))
	assert.Equal(t, time.Minute, backoff.Exponential(time.Second, time.Minute, 100), "the exponential delay caps at one minute")
}

// TestExponentialFirstRungIsNeverClamped proves the first attempt is always the
// base even when the base already exceeds the cap: only rungs past the first are
// clamped to maxDelay.
func TestExponentialFirstRungIsNeverClamped(t *testing.T) {
	t.Parallel()

	const base = time.Second
	const ceiling = 500 * time.Millisecond

	assert.Equal(t, base, backoff.Exponential(base, ceiling, 1), "the first rung is the unclamped base")
	assert.Equal(t, ceiling, backoff.Exponential(base, ceiling, 2), "every rung after the first saturates at the sub-base cap")
}
