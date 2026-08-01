// Package backoff computes retry delay ladders shared across services.
//
// It is deliberately tiny and dependency-free: a single exponential schedule
// that callers layer their own policy on top of — a fixed base/ceiling, an
// attempt counter, and (if they want it) jitter applied to the returned delay.
package backoff

import "time"

// Exponential returns the delay for a retry attempt on an exponential ladder:
// base for the first attempt, doubling once per attempt past the first, capped
// at maxDelay. An attempt below 1 is treated as 1.
//
// The doubling short-circuits the moment it reaches the cap, so a large attempt
// returns maxDelay without looping needlessly or overflowing the duration. The
// first attempt is never clamped — it is always base, even when base already
// exceeds maxDelay — so a caller reads a recognizable first rung before the cap
// takes over.
func Exponential(base, maxDelay time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for range attempt - 1 {
		delay *= 2
		if delay >= maxDelay {
			return maxDelay
		}
	}
	return delay
}
