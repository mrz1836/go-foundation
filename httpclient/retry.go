package httpclient

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"

	"github.com/mrz1836/go-foundation/backoff"
)

// Retry defaults, used for a RetryPolicy field left at zero.
const (
	// DefaultRetryAttempts is the total number of attempts, the first included.
	DefaultRetryAttempts = 3

	// DefaultRetryBaseDelay is the delay before the first retry, before jitter.
	DefaultRetryBaseDelay = 100 * time.Millisecond

	// DefaultRetryMaxDelay caps every delay, and the Retry-After a server may
	// ask for.
	DefaultRetryMaxDelay = 2 * time.Second
)

// maxRetryAfterSeconds caps a parsed Retry-After, far above any MaxDelay.
const maxRetryAfterSeconds = 1 << 31

// drainLimit is how much of a discarded response body is read so its
// connection can be reused.
const drainLimit = 64 << 10

// RetryPolicy configures WithRetry. A zero field takes its default.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, the first included. Zero
	// means DefaultRetryAttempts; one means no retry.
	MaxAttempts int

	// BaseDelay is the delay before the first retry. Each later retry doubles
	// it, up to MaxDelay, and each delay is jittered to between half and all of
	// its value. Zero means DefaultRetryBaseDelay.
	BaseDelay time.Duration

	// MaxDelay caps each delay. A Retry-After longer than MaxDelay ends the
	// retries, and the response that asked for it is returned. Zero means
	// DefaultRetryMaxDelay.
	MaxDelay time.Duration

	// Methods are the request methods that may be retried. Empty means the
	// idempotent methods: GET, HEAD, OPTIONS, TRACE, PUT, and DELETE. A request
	// with a body is retried only when it has GetBody, which
	// http.NewRequestWithContext sets for a *bytes.Buffer, *bytes.Reader, or
	// *strings.Reader body.
	Methods []string

	// Statuses are the response statuses that are retried. Empty means 429,
	// 502, 503, and 504. A transport error is retried unless it is permanent: a
	// refused scheme, host, or address, an oversized body, a certificate
	// failure, a host that does not resolve, or a canceled context.
	Statuses []int
}

// resolve fills p's defaults and refuses invalid values.
func (p *RetryPolicy) resolve() error {
	if p.MaxAttempts < 0 || p.BaseDelay < 0 || p.MaxDelay < 0 {
		return fmt.Errorf("%w: negative retry attempts or delay", ErrInvalidConfig)
	}
	p.MaxAttempts = cmp.Or(p.MaxAttempts, DefaultRetryAttempts)
	p.BaseDelay = cmp.Or(p.BaseDelay, DefaultRetryBaseDelay)
	p.MaxDelay = cmp.Or(p.MaxDelay, DefaultRetryMaxDelay)
	if p.MaxDelay < p.BaseDelay {
		return fmt.Errorf("%w: retry MaxDelay is below BaseDelay", ErrInvalidConfig)
	}
	if err := p.resolveMethods(); err != nil {
		return err
	}

	return p.resolveStatuses()
}

// resolveMethods fills the default methods and refuses an invalid one.
func (p *RetryPolicy) resolveMethods() error {
	if len(p.Methods) == 0 {
		p.Methods = []string{
			http.MethodGet, http.MethodHead, http.MethodOptions,
			http.MethodTrace, http.MethodPut, http.MethodDelete,
		}

		return nil
	}
	methods := make([]string, 0, len(p.Methods))
	for _, m := range p.Methods {
		if !validMethod(m) {
			return fmt.Errorf("%w: invalid retry method %q", ErrInvalidConfig, m)
		}
		methods = append(methods, strings.ToUpper(m))
	}
	p.Methods = methods

	return nil
}

// resolveStatuses fills the default statuses and refuses one outside 100–599.
func (p *RetryPolicy) resolveStatuses() error {
	if len(p.Statuses) == 0 {
		p.Statuses = []int{
			http.StatusTooManyRequests, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout,
		}

		return nil
	}
	for _, s := range p.Statuses {
		if s < 100 || s > 599 {
			return fmt.Errorf("%w: invalid retry status %d", ErrInvalidConfig, s)
		}
	}
	p.Statuses = slices.Clone(p.Statuses)

	return nil
}

// eligible reports whether req may be retried at all.
func (p *RetryPolicy) eligible(req *http.Request) bool {
	if !slices.Contains(p.Methods, req.Method) {
		return false
	}
	hasBody := req.Body != nil && req.Body != http.NoBody

	return !hasBody || req.GetBody != nil
}

// delay returns how long to wait before attempt+1, and false when there is no
// next attempt: the attempts are spent, the outcome is final, or the server
// asked for a wait longer than MaxDelay.
func (p *RetryPolicy) delay(resp *http.Response, err error, attempt int) (time.Duration, bool) {
	if attempt >= p.MaxAttempts {
		return 0, false
	}
	if err != nil {
		return p.backoff(attempt), retryableError(err)
	}
	if !slices.Contains(p.Statuses, resp.StatusCode) {
		return 0, false
	}
	if wait, ok := retryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
		return wait, wait <= p.MaxDelay
	}

	return p.backoff(attempt), true
}

// backoff returns the jittered exponential delay after attempt.
func (p *RetryPolicy) backoff(attempt int) time.Duration {
	d := backoff.Exponential(p.BaseDelay, p.MaxDelay, attempt)
	half := d / 2

	// Jitter needs no cryptographic randomness.
	return half + rand.N(d-half+1) //nolint:gosec // G404: jitter, not a secret
}

// retryAfter parses a Retry-After header, in seconds or as an HTTP date, and
// reports whether it held one.
func retryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
		if secs < 0 {
			return 0, false
		}

		// Cap at about 68 years, so the conversion cannot overflow.
		return time.Duration(min(secs, maxRetryAfterSeconds)) * time.Second, true
	}
	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}

	return max(at.Sub(now), 0), true
}

// retryableError reports whether a transport error may succeed on a retry.
func retryableError(err error) bool {
	permanent := []error{
		context.Canceled, context.DeadlineExceeded,
		ErrSchemeNotAllowed, ErrHostNotAllowed, ErrAddressBlocked, ErrResponseTooLarge,
	}
	for _, target := range permanent {
		if errors.Is(err, target) {
			return false
		}
	}

	return !certificateError(err) && !hostNotFound(err)
}

// certificateError reports whether err is a TLS certificate failure, which a
// retry cannot fix.
func certificateError(err error) bool {
	var verify *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError

	return errors.As(err, &verify) || errors.As(err, &unknown) ||
		errors.As(err, &hostname) || errors.As(err, &invalid)
}

// hostNotFound reports whether err is a DNS answer that the host does not
// exist.
func hostNotFound(err error) bool {
	var dnsErr *net.DNSError

	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// rewind returns a copy of req with a fresh body for another attempt.
func rewind(req *http.Request) (*http.Request, error) {
	next := req.Clone(req.Context())
	if req.GetBody == nil {
		return next, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("httpclient: replay request body: %w", err)
	}
	next.Body = body

	return next, nil
}

// wait sleeps for d, or until ctx is done.
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("httpclient: waiting to retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// discard drains a bounded part of resp's body and closes it, so the
// connection can be reused.
func discard(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, resp.Body, drainLimit)
	_ = resp.Body.Close()
}

// validMethod reports whether m is a valid HTTP method token.
func validMethod(m string) bool {
	return m != "" && strings.IndexFunc(m, func(r rune) bool {
		return r > 0x7f || !httpguts.IsTokenRune(r)
	}) < 0
}
