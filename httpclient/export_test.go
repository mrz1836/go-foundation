package httpclient

import (
	"net/http"
	"time"
)

// BaseTransport returns the net/http transport under a client's enforcing
// transport, so tests can inspect how it was configured.
func BaseTransport(rt http.RoundTripper) *http.Transport {
	t, ok := rt.(*transport)
	if !ok {
		return nil
	}

	return t.base
}

// RetryAfter exposes retryAfter to tests.
func RetryAfter(v string, now time.Time) (time.Duration, bool) {
	return retryAfter(v, now)
}

// Backoff returns the jittered delay policy would wait after attempt, with its
// defaults filled in.
func Backoff(policy RetryPolicy, attempt int) (time.Duration, error) {
	if err := policy.resolve(); err != nil {
		return 0, err
	}

	return policy.backoff(attempt), nil
}

// Retryable exposes retryableError to tests.
func Retryable(err error) bool {
	return retryableError(err)
}
