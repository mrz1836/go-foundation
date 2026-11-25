package httpclient_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

// fakeSecret stands in for a credential; its appearance in any output is a
// leak.
const fakeSecret = "sk-test-5ecr3t-do-not-print" //nolint:gosec // G101: a fake value the tests search for

// newTLSServer starts a TLS test server for h on 127.0.0.1, closed when the
// test ends.
func newTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)

	return srv
}

// ok answers every request with 200 and a small JSON body.
func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

// trust returns an option that trusts srv's certificate.
func trust(t *testing.T, srv *httptest.Server) httpclient.Option {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	return httpclient.WithTLSConfig(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
}

// loopbackOnly is an address guard that admits only loopback addresses, so a
// guarded client can reach a local test server.
func loopbackOnly(addr netip.Addr) bool {
	return addr.Unmap().IsLoopback()
}

// hostOf returns the host name of srv's URL, "127.0.0.1".
func hostOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return u.Hostname()
}

// portOf returns the port of srv's URL.
func portOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return u.Port()
}

// pinnedTo returns a Pinned client for srv that trusts its certificate and
// admits loopback addresses.
func pinnedTo(t *testing.T, srv *httptest.Server, opts ...httpclient.Option) *http.Client {
	t.Helper()
	opts = append([]httpclient.Option{trust(t, srv), httpclient.WithAddressGuard(loopbackOnly)}, opts...)
	client, err := httpclient.Pinned(hostOf(t, srv), opts...)
	require.NoError(t, err)

	return client
}

// fetch sends a GET to rawURL through client and returns the response status,
// closing the body. The status is zero when the request fails.
func fetch(t *testing.T, client *http.Client, rawURL string) (int, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// newPlainServer starts a plain-HTTP test server that answers like ok, and
// returns its URL.
func newPlainServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(ok())
	t.Cleanup(srv.Close)

	return srv.URL
}

// newTLS12Server starts a TLS test server that speaks TLS 1.2 only.
func newTLS12Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(ok())
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12} //nolint:gosec // G402: the test needs a server without TLS 1.3
	// The refused handshake is expected; keep it out of the test output.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv
}
