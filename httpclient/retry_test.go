package httpclient_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

// fastRetry is a retry policy with delays short enough for tests.
func fastRetry(p httpclient.RetryPolicy) httpclient.Option {
	p.BaseDelay = time.Millisecond
	if p.MaxDelay == 0 {
		p.MaxDelay = 5 * time.Millisecond
	}

	return httpclient.WithRetry(p)
}

// sequence answers each request with the next status in statuses, and 200
// once they run out. It counts the requests it saw and records their bodies.
type sequence struct {
	mu       sync.Mutex
	statuses []int
	header   http.Header
	bodies   []string
}

// ServeHTTP answers with the next status.
func (s *sequence) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := io.ReadAll(r.Body)
	s.bodies = append(s.bodies, string(b))
	for k, v := range s.header {
		w.Header()[k] = v
	}
	status := http.StatusOK
	if len(s.statuses) > 0 {
		status, s.statuses = s.statuses[0], s.statuses[1:]
	}
	w.WriteHeader(status)
}

// calls returns how many requests s saw.
func (s *sequence) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.bodies)
}

func TestRetryRetriesARetryableStatus(t *testing.T) {
	seq := &sequence{statuses: []int{http.StatusServiceUnavailable, http.StatusBadGateway}}
	srv := newTLSServer(t, seq)

	var attempts []int
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{}),
		httpclient.WithObserver(func(e httpclient.Event) { attempts = append(attempts, e.Attempt) }))

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 3, seq.calls())
	assert.Equal(t, []int{1, 2, 3}, attempts)
}

func TestRetryReturnsTheLastResponseWhenSpent(t *testing.T) {
	seq := &sequence{statuses: []int{503, 503, 503}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{MaxAttempts: 2}))

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, 2, seq.calls())
}

func TestRetryLeavesOtherStatusesAlone(t *testing.T) {
	seq := &sequence{statuses: []int{http.StatusInternalServerError}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{}))

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, 1, seq.calls())
}

func TestRetryHonorsRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		wantStatus int
		wantCalls  int
	}{
		{name: "within the max delay", retryAfter: "0", wantStatus: http.StatusOK, wantCalls: 2},
		{name: "longer than the max delay", retryAfter: "120", wantStatus: http.StatusTooManyRequests, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := &sequence{
				statuses: []int{http.StatusTooManyRequests},
				header:   http.Header{"Retry-After": {tt.retryAfter}},
			}
			srv := newTLSServer(t, seq)
			client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{}))

			status, err := fetch(t, client, srv.URL)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantCalls, seq.calls())
		})
	}
}

func TestRetrySkipsNonIdempotentMethodsByDefault(t *testing.T) {
	seq := &sequence{statuses: []int{503}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{}))

	status := post(t, client, srv.URL, bytes.NewReader([]byte("payload")))
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, 1, seq.calls())
}

func TestRetryReplaysTheBodyWhenPOSTIsAllowed(t *testing.T) {
	seq := &sequence{statuses: []int{503, 503}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{Methods: []string{"post"}}))

	status := post(t, client, srv.URL, strings.NewReader("payload"))
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, []string{"payload", "payload", "payload"}, seq.bodies)
}

func TestRetrySkipsABodyThatCannotBeReplayed(t *testing.T) {
	seq := &sequence{statuses: []int{503}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{Methods: []string{http.MethodPost}}))

	// A plain io.Reader gets no GetBody.
	status := post(t, client, srv.URL, io.MultiReader(strings.NewReader("payload")))
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, 1, seq.calls())
}

// post sends body in a POST through client and returns the status.
func post(t *testing.T, client *http.Client, rawURL string, body io.Reader) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, rawURL, body)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	return resp.StatusCode
}

func TestRetryRetriesATransportError(t *testing.T) {
	var calls atomic.Int32
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			// Drop the connection without answering.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}

			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{}))

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int32(2), calls.Load())
}

func TestRetryNeverRetriesAPermanentError(t *testing.T) {
	srv := newTLSServer(t, ok())

	var attempts atomic.Int32
	client, err := httpclient.Pinned(hostOf(t, srv), trust(t, srv), fastRetry(httpclient.RetryPolicy{}),
		httpclient.WithObserver(func(httpclient.Event) { attempts.Add(1) }))
	require.NoError(t, err)

	_, err = fetch(t, client, srv.URL)
	require.ErrorIs(t, err, httpclient.ErrAddressBlocked)
	assert.Equal(t, int32(1), attempts.Load())
}

func TestRetryableErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "connection reset", err: &net.OpError{Op: "read", Err: syscall.ECONNRESET}, want: true},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, want: true},
		{name: "temporary DNS failure", err: &net.DNSError{Err: "timeout", IsTemporary: true}, want: true},
		{name: "unknown host", err: &net.DNSError{Err: "no such host", IsNotFound: true}, want: false},
		{name: "canceled", err: fmt.Errorf("wrapped: %w", context.Canceled), want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: false},
		{name: "blocked address", err: &net.OpError{Op: "dial", Err: httpclient.ErrAddressBlocked}, want: false},
		{name: "host not allowed", err: httpclient.ErrHostNotAllowed, want: false},
		{name: "scheme not allowed", err: httpclient.ErrSchemeNotAllowed, want: false},
		{name: "body too large", err: httpclient.ErrResponseTooLarge, want: false},
		{name: "unknown authority", err: x509.UnknownAuthorityError{}, want: false},
		{name: "certificate verification", err: &tls.CertificateVerificationError{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, httpclient.Retryable(tt.err))
		})
	}
}

func TestRetryWaitStopsWhenTheContextEnds(t *testing.T) {
	seq := &sequence{statuses: []int{503}}
	srv := newTLSServer(t, seq)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client := pinnedTo(t, srv,
		httpclient.WithRetry(httpclient.RetryPolicy{BaseDelay: time.Hour, MaxDelay: time.Hour}),
		httpclient.WithObserver(func(httpclient.Event) { cancel() }))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), time.Minute)
	assert.Equal(t, 1, seq.calls())
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name   string
		value  string
		want   time.Duration
		wantOK bool
	}{
		{name: "empty", value: "", wantOK: false},
		{name: "seconds", value: "7", want: 7 * time.Second, wantOK: true},
		{name: "seconds with spaces", value: " 3 ", want: 3 * time.Second, wantOK: true},
		{name: "negative", value: "-1", wantOK: false},
		{name: "huge", value: "99999999999999", want: (1 << 31) * time.Second, wantOK: true},
		{name: "date", value: now.Add(90 * time.Second).Format(http.TimeFormat), want: 90 * time.Second, wantOK: true},
		{name: "past date", value: now.Add(-time.Hour).Format(http.TimeFormat), want: 0, wantOK: true},
		{name: "garbage", value: "soon", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := httpclient.RetryAfter(tt.value, now)
			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRetryBackoffIsJitteredAndCapped(t *testing.T) {
	policy := httpclient.RetryPolicy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}

	for attempt := 1; attempt <= 6; attempt++ {
		full := min(100*time.Millisecond<<(attempt-1), time.Second)
		for range 50 {
			d, err := httpclient.Backoff(policy, attempt)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, d, full/2)
			assert.LessOrEqual(t, d, full)
		}
	}
}

func TestRetryCustomStatuses(t *testing.T) {
	seq := &sequence{statuses: []int{http.StatusInternalServerError}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{Statuses: []int{http.StatusInternalServerError}}))

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 2, seq.calls())
}

func TestRetryStopsWhenTheBodyCannotBeReplayed(t *testing.T) {
	seq := &sequence{statuses: []int{503}}
	srv := newTLSServer(t, seq)
	client := pinnedTo(t, srv, fastRetry(httpclient.RetryPolicy{Methods: []string{http.MethodPut}}))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL, strings.NewReader("payload"))
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) { return nil, io.ErrUnexpectedEOF }
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "the last response is returned")
	assert.Equal(t, 1, seq.calls())
}
