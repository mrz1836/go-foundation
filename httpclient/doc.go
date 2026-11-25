// Package httpclient builds outbound HTTP clients that are safe by default.
// Every client is a standard *http.Client (or an http.RoundTripper, from
// NewTransport), so it plugs into any SDK that accepts one.
//
// A client enforces its rules in its transport, so every request, and every
// redirect hop, is checked:
//
//   - a scheme allowlist (http and https, or HTTPS only);
//   - an optional host allowlist;
//   - an optional dial-time address guard, judged on the address actually
//     dialed, so a DNS answer that changes between a check and a connect
//     cannot reach a private, loopback, link-local, metadata, or other
//     non-public address (see IsPublicAddress);
//   - per-phase timeouts (dial, TLS handshake, response header, idle, overall);
//   - a bounded redirect policy, with an optional hook that re-checks each hop;
//   - limits on the response header and body size;
//   - no proxy unless one is enabled from the environment;
//   - a minimum TLS version;
//   - opt-in retries with exponential backoff and jitter, which honor
//     Retry-After and replay a request body through GetBody;
//   - an observer hook that sees each attempt's method, host, status,
//     attempt number, duration, and error, never its headers, body, or query.
//
// Two presets cover the common cases:
//
//   - Pinned(host) calls one known API: HTTPS only, that host only, no
//     redirects followed, no proxy, and the address guard on.
//   - Guarded() calls destinations that are not known in advance: HTTPS only,
//     at most three redirects, no proxy, and the address guard on.
//
// Options passed to a preset are applied after its own, so they adjust it. New
// refuses a configuration that contradicts itself, such as an address guard
// together with an environment proxy, which would judge the proxy's address
// instead of the destination's.
//
// The helpers GetJSON, PostJSON, DoJSON, DecodeJSON, and ReadBody read bounded
// bodies and return a *StatusError, which never holds the response body, for
// a status outside 2xx. Errors from this package never include a URL's
// userinfo or query string; RedactError does the same for an error returned by
// (*http.Client).Do.
package httpclient
