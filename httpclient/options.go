package httpclient

import (
	"crypto/tls"
	"net/http"
	"net/netip"
	"time"
)

// Option configures a client built by New, NewTransport, Pinned, or Guarded.
// Options apply in order, so a later option overrides an earlier one.
type Option func(*config)

// WithTimeout sets the overall limit on a request made through the
// *http.Client, from the first dial to reading the last byte of the body, and
// including redirects and retries. Zero means no overall limit. It does not
// apply to a bare transport from NewTransport.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithDialTimeout sets the limit on establishing one TCP connection. Zero
// means no limit.
func WithDialTimeout(d time.Duration) Option {
	return func(c *config) { c.dialTimeout = d }
}

// WithTLSHandshakeTimeout sets the limit on one TLS handshake. Zero means no
// limit.
func WithTLSHandshakeTimeout(d time.Duration) Option {
	return func(c *config) { c.tlsHandshakeTimeout = d }
}

// WithResponseHeaderTimeout sets the limit on waiting for the response headers
// once the request is written. Zero means no limit.
func WithResponseHeaderTimeout(d time.Duration) Option {
	return func(c *config) { c.responseHeaderTimeout = d }
}

// WithIdleConnTimeout sets how long an idle keep-alive connection stays open.
// Zero means no limit.
func WithIdleConnTimeout(d time.Duration) Option {
	return func(c *config) { c.idleConnTimeout = d }
}

// WithMaxRedirects sets how many redirects a request follows. With zero, a
// redirect is not followed: the client returns the 3xx response itself. Past n
// redirects, the request fails with ErrTooManyRedirects. Every hop is still
// checked against the scheme and host rules. It does not apply to a bare
// transport from NewTransport.
func WithMaxRedirects(n int) Option {
	return func(c *config) { c.maxRedirects = n }
}

// WithNoRedirects is WithMaxRedirects(0): a redirect is returned, not
// followed.
func WithNoRedirects() Option {
	return WithMaxRedirects(0)
}

// WithRedirectCheck adds check, which runs for each redirect the client would
// follow, after the redirect count is checked. A non-nil error stops the
// redirect and fails the request with it. req is the next hop, and req's
// context is the original request's. It needs WithMaxRedirects above zero.
func WithRedirectCheck(check func(req *http.Request, via []*http.Request) error) Option {
	return func(c *config) { c.redirectCheck = check }
}

// WithMaxResponseHeaderBytes limits the size of the response headers. Zero
// uses net/http's default (1 MiB).
func WithMaxResponseHeaderBytes(n int64) Option {
	return func(c *config) { c.maxHeaderBytes = n }
}

// WithMaxBodySize limits every response body to n bytes. A response whose
// Content-Length is larger fails before its body is read, and a body that
// grows past n fails with ErrResponseTooLarge when read. Zero means no limit.
func WithMaxBodySize(n int64) Option {
	return func(c *config) { c.maxBodyBytes = n }
}

// WithAllowedSchemes sets the URL schemes a request, or a redirect hop, may
// use: "http", "https", or both.
func WithAllowedSchemes(schemes ...string) Option {
	return func(c *config) { c.schemes = schemes }
}

// WithHTTPSOnly allows only the https scheme.
func WithHTTPSOnly() Option {
	return WithAllowedSchemes(schemeHTTPS)
}

// WithAllowedHosts limits requests, and every redirect hop, to hosts. Each
// entry is a bare host name or IP literal, matched exactly and without regard
// to case or port. At least one host is required.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *config) {
		c.hosts = hosts
		c.hostsSet = true
	}
}

// WithAddressGuard refuses every dial whose resolved address allow does not
// accept. It runs on the address actually dialed, after DNS resolution, so a
// changed DNS answer cannot slip past it. The address is passed as dialed: it
// may be IPv4-mapped or carry a zone. IsPublicAddress is the usual guard.
func WithAddressGuard(allow func(netip.Addr) bool) Option {
	return func(c *config) {
		c.guard = allow
		c.guardSet = true
	}
}

// WithoutAddressGuard removes the address guard, so any resolved address may
// be dialed.
func WithoutAddressGuard() Option {
	return func(c *config) {
		c.guard = nil
		c.guardSet = false
	}
}

// WithProxyFromEnvironment sends requests through the proxy that the
// HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables name. Without
// it, no proxy is used. It cannot be combined with an address guard, which
// would judge the proxy's address instead of the destination's.
func WithProxyFromEnvironment() Option {
	return func(c *config) { c.proxyFromEnv = true }
}

// WithTLSConfig sets the TLS client configuration, such as the root CAs or a
// client certificate. It is cloned, so later changes to cfg do not affect the
// client. The minimum TLS version is raised to the client's minimum (TLS 1.2
// unless WithMinTLSVersion says otherwise).
func WithTLSConfig(cfg *tls.Config) Option {
	return func(c *config) { c.tlsConfig = cfg }
}

// WithMinTLSVersion sets the lowest TLS version the client accepts, such as
// tls.VersionTLS13. The default is tls.VersionTLS12.
func WithMinTLSVersion(version uint16) Option {
	return func(c *config) { c.minTLSVersion = version }
}

// WithUserAgent sets the User-Agent header on every request that does not set
// its own.
func WithUserAgent(userAgent string) Option {
	return func(c *config) { c.userAgent = userAgent }
}

// WithHeader sets a static header on every request that does not set it
// itself, including every redirect hop. A static header must not carry a
// credential: net/http drops a request's own Authorization and Cookie headers
// on a redirect to another domain, but a static header is sent to every host
// the client reaches. Set credentials on the request instead.
func WithHeader(key, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = make(http.Header)
		}
		c.headers.Add(key, value)
	}
}

// WithRetry turns on retries with policy. Without it, every request is sent
// once.
func WithRetry(policy RetryPolicy) Option {
	return func(c *config) { c.retry = &policy }
}

// WithObserver calls observe once for each attempt, after it completes. The
// Event holds the method, host, status, attempt number, duration, and error,
// never the headers, body, or query string. observe runs on the request's
// goroutine and must be safe for concurrent use.
func WithObserver(observe func(Event)) Option {
	return func(c *config) { c.observer = observe }
}
