package httpclient

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"slices"
	"time"
)

// Connection-pool settings, as net/http's DefaultTransport uses.
const (
	maxIdleConns          = 100
	expectContinueTimeout = time.Second
)

// New returns an *http.Client configured by opts, or an error wrapping
// ErrInvalidConfig when the options are invalid or contradict each other.
//
// With no options the client allows http and https to any host, uses no
// proxy, requires TLS 1.2 or later, follows up to DefaultMaxRedirects
// redirects, and bounds each phase with the Default* timeouts. Add the guards a
// caller needs, or start from Pinned or Guarded.
func New(opts ...Option) (*http.Client, error) {
	c := newConfig(opts)
	if err := c.validate(); err != nil {
		return nil, err
	}

	return &http.Client{
		Transport:     c.transport(),
		CheckRedirect: c.checkRedirect(),
		Timeout:       c.timeout,
	}, nil
}

// NewTransport returns the http.RoundTripper New would use, for an SDK that
// takes a transport rather than a client. The scheme and host rules, address
// guard, phase timeouts, size limits, headers, retries, and observer all
// apply. The overall timeout and the redirect policy belong to an
// *http.Client, so a bare transport does not apply them.
func NewTransport(opts ...Option) (http.RoundTripper, error) {
	c := newConfig(opts)
	if err := c.validate(); err != nil {
		return nil, err
	}

	return c.transport(), nil
}

// Pinned returns a client for one known API at host: HTTPS only, host only,
// redirects returned rather than followed, no proxy, the address guard
// (IsPublicAddress) on, and the Guarded* timeouts and size limits. host is a
// bare host name or IP literal, such as "api.example.com". opts adjust the
// preset, except that the host allowlist cannot change.
func Pinned(host string, opts ...Option) (*http.Client, error) {
	h, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	preset := append(guardedOptions(), WithNoRedirects(), WithAllowedHosts(h), pin(h))

	return New(append(preset, opts...)...)
}

// Guarded returns a client for destinations that are not known in advance,
// such as URLs a user configures: HTTPS only, any host that resolves to a
// public address (IsPublicAddress), up to GuardedMaxRedirects redirects each
// re-checked, no proxy, and the Guarded* timeouts and size limits. opts adjust
// the preset; add WithAllowedHosts or WithRedirectCheck to narrow it further.
func Guarded(opts ...Option) (*http.Client, error) {
	return New(append(guardedOptions(), opts...)...)
}

// pin records the host a Pinned client is limited to.
func pin(host string) Option {
	return func(c *config) { c.pinnedHost = host }
}

// transport builds the enforcing transport from c, which has been validated.
func (c *config) transport() *transport {
	dialer := &net.Dialer{Timeout: c.dialTimeout, KeepAlive: keepAlive}
	if c.guard != nil {
		dialer.Control = dialControl(c.guard)
	}
	base := &http.Transport{
		DialContext:            dialer.DialContext,
		TLSClientConfig:        c.tls(),
		TLSHandshakeTimeout:    c.tlsHandshakeTimeout,
		ResponseHeaderTimeout:  c.responseHeaderTimeout,
		IdleConnTimeout:        c.idleConnTimeout,
		MaxResponseHeaderBytes: c.maxHeaderBytes,
		MaxIdleConns:           maxIdleConns,
		ExpectContinueTimeout:  expectContinueTimeout,
		ForceAttemptHTTP2:      true,
	}
	if c.proxyFromEnv {
		base.Proxy = http.ProxyFromEnvironment
	}

	t := &transport{
		base:      base,
		schemes:   slices.Clone(c.schemes),
		userAgent: c.userAgent,
		headers:   c.headers.Clone(),
		maxBody:   c.maxBodyBytes,
		retry:     c.retry,
		observer:  c.observer,
	}
	if c.hostsSet {
		t.hosts = make(map[string]struct{}, len(c.hosts))
		for _, h := range c.hosts {
			t.hosts[h] = struct{}{}
		}
	}

	return t
}

// tls returns the TLS client configuration: a clone of the caller's, or a new
// one, with the minimum version raised to the client's minimum.
func (c *config) tls() *tls.Config {
	if c.tlsConfig == nil {
		return &tls.Config{MinVersion: c.minTLSVersion}
	}
	cfg := c.tlsConfig.Clone()
	cfg.MinVersion = max(cfg.MinVersion, c.minTLSVersion)

	return cfg
}

// checkRedirect returns the client's redirect policy: return the redirect
// response when no redirects are allowed, fail past the limit, and otherwise
// run the caller's check. The transport checks each hop's scheme and host.
func (c *config) checkRedirect() func(*http.Request, []*http.Request) error {
	limit, check := c.maxRedirects, c.redirectCheck

	return func(req *http.Request, via []*http.Request) error {
		if limit == 0 {
			return http.ErrUseLastResponse
		}
		if len(via) > limit {
			return fmt.Errorf("%w: more than %d", ErrTooManyRedirects, limit)
		}
		if check != nil {
			return check(req, via)
		}

		return nil
	}
}
