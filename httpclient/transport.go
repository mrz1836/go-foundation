package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Event describes one attempt of a request, for WithObserver. It never holds
// the request's or response's headers or body, or the URL's path, userinfo, or
// query string.
type Event struct {
	// Method is the request's HTTP method.
	Method string

	// Host is the request URL's host, with its port when the URL has one.
	Host string

	// Status is the response's status code, or zero when there was no
	// response.
	Status int

	// Attempt is the attempt number, starting at one.
	Attempt int

	// Duration is how long the attempt took, up to the response headers.
	Duration time.Duration

	// Err is the attempt's error, or nil.
	Err error
}

// transport is the http.RoundTripper that enforces a client's rules.
type transport struct {
	base      *http.Transport
	schemes   []string
	hosts     map[string]struct{} // nil allows any host
	userAgent string
	headers   http.Header
	maxBody   int64
	retry     *RetryPolicy
	observer  func(Event)
}

// RoundTrip checks req against the scheme and host rules, adds the static
// headers, and sends it, retrying when the retry policy allows.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.check(req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		t.observe(req, 1, nil, 0, err)

		return nil, err
	}
	req = t.withHeaders(req)
	if t.retry == nil || !t.retry.eligible(req) {
		return t.attempt(req, 1)
	}

	return t.withRetries(req)
}

// CloseIdleConnections closes the idle connections of the underlying
// transport. (*http.Client).CloseIdleConnections calls it.
func (t *transport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
}

// check refuses a URL whose scheme or host the client does not allow.
func (t *transport) check(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("%w: no URL", ErrHostNotAllowed)
	}
	if !slices.Contains(t.schemes, strings.ToLower(u.Scheme)) {
		return fmt.Errorf("%w: %q to %s", ErrSchemeNotAllowed, u.Scheme, u.Host)
	}
	if t.hosts == nil {
		return nil
	}
	if _, ok := t.hosts[requestHost(u)]; !ok {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, u.Host)
	}

	return nil
}

// withHeaders returns req with the static headers it does not set itself. It
// clones req rather than change the caller's request.
func (t *transport) withHeaders(req *http.Request) *http.Request {
	if t.userAgent == "" && len(t.headers) == 0 {
		return req
	}
	out := req.Clone(req.Context())
	if out.Header == nil {
		out.Header = make(http.Header)
	}
	if _, ok := out.Header["User-Agent"]; !ok && t.userAgent != "" {
		out.Header.Set("User-Agent", t.userAgent)
	}
	for key, values := range t.headers {
		if _, ok := out.Header[key]; !ok {
			out.Header[key] = slices.Clone(values)
		}
	}

	return out
}

// withRetries sends req until it succeeds, its outcome is final, or the retry
// policy is spent.
func (t *transport) withRetries(req *http.Request) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := t.attempt(req, attempt)
		delay, again := t.retry.delay(resp, err, attempt)
		if !again {
			return resp, err
		}
		next, rewindErr := rewind(req)
		if rewindErr != nil {
			return resp, err
		}
		discard(resp)
		if waitErr := wait(req.Context(), delay); waitErr != nil {
			return nil, waitErr
		}
		req = next
	}
}

// attempt sends req once through the base transport, applies the body limit,
// and reports the attempt to the observer.
func (t *transport) attempt(req *http.Request, n int) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	if err == nil {
		resp, err = t.limit(req, resp)
	}
	t.observe(req, n, resp, time.Since(start), err)

	return resp, err
}

// limit fails a response that declares a body over the limit, and wraps the
// body of any other so reading past the limit fails.
func (t *transport) limit(req *http.Request, resp *http.Response) (*http.Response, error) {
	if t.maxBody <= 0 || resp.Body == nil || resp.Body == http.NoBody {
		return resp, nil
	}
	if resp.ContentLength > t.maxBody {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("%w: %s declared %d bytes, over the %d-byte limit",
			ErrResponseTooLarge, req.URL.Host, resp.ContentLength, t.maxBody)
	}
	resp.Body = &limitedBody{body: resp.Body, remaining: t.maxBody}

	return resp, nil
}

// observe reports one attempt to the observer, when there is one.
func (t *transport) observe(req *http.Request, n int, resp *http.Response, d time.Duration, err error) {
	if t.observer == nil {
		return
	}
	event := Event{Method: req.Method, Attempt: n, Duration: d, Err: err}
	if event.Method == "" {
		event.Method = http.MethodGet
	}
	if req.URL != nil {
		event.Host = req.URL.Host
	}
	if resp != nil {
		event.Status = resp.StatusCode
	}
	t.observer(event)
}

// limitedBody is a response body that fails with ErrResponseTooLarge once
// more than its limit has been read.
type limitedBody struct {
	body      io.ReadCloser
	remaining int64
	err       error
}

// Read reads from the body, failing once the body passes the limit.
func (l *limitedBody) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	// Read one byte past the limit, to tell a body that ends at the limit
	// from one that goes past it. Comparing before adding one keeps a limit
	// near math.MaxInt64 from overflowing.
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining+1]
	}
	n, err := l.body.Read(p)
	if int64(n) > l.remaining {
		n = int(l.remaining)
		l.remaining = 0
		l.err = ErrResponseTooLarge

		return n, l.err
	}
	l.remaining -= int64(n)

	// The body's own error, io.EOF included, passes through unchanged.
	return n, err
}

// Close closes the body.
func (l *limitedBody) Close() error {
	return l.body.Close()
}
