package httpclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

// URL schemes a client may allow.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// Defaults for New. Pinned and Guarded use the tighter Guarded* values.
const (
	// DefaultDialTimeout bounds one TCP connect.
	DefaultDialTimeout = 10 * time.Second

	// DefaultTLSHandshakeTimeout bounds one TLS handshake.
	DefaultTLSHandshakeTimeout = 10 * time.Second

	// DefaultResponseHeaderTimeout bounds the wait for response headers.
	DefaultResponseHeaderTimeout = 30 * time.Second

	// DefaultIdleConnTimeout is how long an idle connection stays open.
	DefaultIdleConnTimeout = 90 * time.Second

	// DefaultTimeout bounds a whole request, redirects and retries included.
	DefaultTimeout = 60 * time.Second

	// DefaultMaxRedirects is how many redirects New follows, as net/http does.
	DefaultMaxRedirects = 10

	// DefaultMaxResponseHeaderBytes limits the response headers.
	DefaultMaxResponseHeaderBytes = 1 << 20
)

// Defaults for Pinned and Guarded.
const (
	// GuardedDialTimeout bounds one TCP connect.
	GuardedDialTimeout = 5 * time.Second

	// GuardedTLSHandshakeTimeout bounds one TLS handshake.
	GuardedTLSHandshakeTimeout = 5 * time.Second

	// GuardedResponseHeaderTimeout bounds the wait for response headers.
	GuardedResponseHeaderTimeout = 15 * time.Second

	// GuardedTimeout bounds a whole request, redirects and retries included.
	GuardedTimeout = 30 * time.Second

	// GuardedMaxRedirects is how many redirects Guarded follows. Pinned
	// follows none.
	GuardedMaxRedirects = 3

	// GuardedMaxResponseHeaderBytes limits the response headers.
	GuardedMaxResponseHeaderBytes = 64 << 10

	// GuardedMaxBodySize limits every response body.
	GuardedMaxBodySize = 10 << 20
)

// keepAlive is the TCP keep-alive period for every connection.
const keepAlive = 30 * time.Second

// config is the resolved set of options.
type config struct {
	dialTimeout           time.Duration
	tlsHandshakeTimeout   time.Duration
	responseHeaderTimeout time.Duration
	idleConnTimeout       time.Duration
	timeout               time.Duration

	maxRedirects  int
	redirectCheck func(*http.Request, []*http.Request) error

	maxHeaderBytes int64
	maxBodyBytes   int64

	schemes    []string
	hosts      []string
	hostsSet   bool
	pinnedHost string

	guard        func(netip.Addr) bool
	guardSet     bool
	proxyFromEnv bool

	tlsConfig     *tls.Config
	minTLSVersion uint16

	userAgent string
	headers   http.Header

	retry    *RetryPolicy
	observer func(Event)
}

// newConfig returns New's defaults with opts applied.
func newConfig(opts []Option) *config {
	c := &config{
		dialTimeout:           DefaultDialTimeout,
		tlsHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		responseHeaderTimeout: DefaultResponseHeaderTimeout,
		idleConnTimeout:       DefaultIdleConnTimeout,
		timeout:               DefaultTimeout,
		maxRedirects:          DefaultMaxRedirects,
		maxHeaderBytes:        DefaultMaxResponseHeaderBytes,
		schemes:               []string{schemeHTTP, schemeHTTPS},
		minTLSVersion:         tls.VersionTLS12,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	return c
}

// guardedOptions returns the options Guarded and Pinned start from.
func guardedOptions() []Option {
	return []Option{
		WithDialTimeout(GuardedDialTimeout),
		WithTLSHandshakeTimeout(GuardedTLSHandshakeTimeout),
		WithResponseHeaderTimeout(GuardedResponseHeaderTimeout),
		WithTimeout(GuardedTimeout),
		WithMaxRedirects(GuardedMaxRedirects),
		WithMaxResponseHeaderBytes(GuardedMaxResponseHeaderBytes),
		WithMaxBodySize(GuardedMaxBodySize),
		WithHTTPSOnly(),
		WithAddressGuard(IsPublicAddress),
	}
}

// validate checks c and returns every problem it finds, joined, each wrapping
// ErrInvalidConfig. It also canonicalizes the schemes and hosts.
func (c *config) validate() error {
	return errors.Join(
		c.validateLimits(),
		c.validateRedirects(),
		c.validateSchemes(),
		c.validateHosts(),
		c.validateGuard(),
		c.validateTLS(),
		c.validateHeaders(),
		c.validateRetry(),
	)
}

// validateLimits refuses negative timeouts and sizes.
func (c *config) validateLimits() error {
	durations := []time.Duration{
		c.dialTimeout, c.tlsHandshakeTimeout, c.responseHeaderTimeout, c.idleConnTimeout, c.timeout,
	}
	for _, d := range durations {
		if d < 0 {
			return fmt.Errorf("%w: negative timeout %s", ErrInvalidConfig, d)
		}
	}
	if c.maxHeaderBytes < 0 || c.maxBodyBytes < 0 {
		return fmt.Errorf("%w: negative size limit", ErrInvalidConfig)
	}

	return nil
}

// validateRedirects refuses a negative redirect count, and a redirect check
// that could never run.
func (c *config) validateRedirects() error {
	if c.maxRedirects < 0 {
		return fmt.Errorf("%w: negative max redirects %d", ErrInvalidConfig, c.maxRedirects)
	}
	if c.redirectCheck != nil && c.maxRedirects == 0 {
		return fmt.Errorf("%w: a redirect check needs WithMaxRedirects above zero", ErrInvalidConfig)
	}

	return nil
}

// validateSchemes refuses an empty or unknown scheme list.
func (c *config) validateSchemes() error {
	if len(c.schemes) == 0 {
		return fmt.Errorf("%w: no allowed schemes", ErrInvalidConfig)
	}
	schemes := make([]string, 0, len(c.schemes))
	for _, s := range c.schemes {
		s = strings.ToLower(s)
		if s != schemeHTTP && s != schemeHTTPS {
			return fmt.Errorf("%w: scheme %q is not http or https", ErrInvalidConfig, s)
		}
		schemes = append(schemes, s)
	}
	c.schemes = schemes

	return nil
}

// validateHosts canonicalizes the allowlist and refuses an invalid or empty
// one, or one that changes a pinned host.
func (c *config) validateHosts() error {
	if !c.hostsSet {
		return nil
	}
	if len(c.hosts) == 0 {
		return fmt.Errorf("%w: WithAllowedHosts needs at least one host", ErrInvalidConfig)
	}
	hosts := make([]string, 0, len(c.hosts))
	for _, h := range c.hosts {
		normalized, err := normalizeHost(h)
		if err != nil {
			return err
		}
		hosts = append(hosts, normalized)
	}
	c.hosts = hosts
	if c.pinnedHost != "" && (len(hosts) != 1 || hosts[0] != c.pinnedHost) {
		return fmt.Errorf("%w: a pinned client cannot change its host allowlist", ErrInvalidConfig)
	}

	return nil
}

// validateGuard refuses a nil guard, and a guard combined with an environment
// proxy.
func (c *config) validateGuard() error {
	if c.guardSet && c.guard == nil {
		return fmt.Errorf("%w: nil address guard; use WithoutAddressGuard", ErrInvalidConfig)
	}
	if c.guard != nil && c.proxyFromEnv {
		return fmt.Errorf("%w: an address guard cannot be combined with an environment proxy, "+
			"which it would judge instead of the destination", ErrInvalidConfig)
	}

	return nil
}

// validateTLS refuses an unknown minimum version, and a maximum below it.
func (c *config) validateTLS() error {
	if c.minTLSVersion < tls.VersionTLS10 || c.minTLSVersion > tls.VersionTLS13 {
		return fmt.Errorf("%w: unknown minimum TLS version %#x", ErrInvalidConfig, c.minTLSVersion)
	}
	if c.tlsConfig != nil && c.tlsConfig.MaxVersion != 0 && c.tlsConfig.MaxVersion < c.minTLSVersion {
		return fmt.Errorf("%w: TLS MaxVersion is below the minimum version", ErrInvalidConfig)
	}

	return nil
}

// validateHeaders refuses a header name or value net/http would reject.
func (c *config) validateHeaders() error {
	if c.userAgent != "" && !httpguts.ValidHeaderFieldValue(c.userAgent) {
		return fmt.Errorf("%w: invalid User-Agent value", ErrInvalidConfig)
	}
	for key, values := range c.headers {
		if !httpguts.ValidHeaderFieldName(key) {
			return fmt.Errorf("%w: invalid header name %q", ErrInvalidConfig, key)
		}
		for _, v := range values {
			if !httpguts.ValidHeaderFieldValue(v) {
				// The value is not quoted: it may be a credential set by mistake.
				return fmt.Errorf("%w: invalid value for header %q", ErrInvalidConfig, key)
			}
		}
	}

	return nil
}

// validateRetry checks the retry policy, when there is one, and fills its
// defaults.
func (c *config) validateRetry() error {
	if c.retry == nil {
		return nil
	}

	return c.retry.resolve()
}
