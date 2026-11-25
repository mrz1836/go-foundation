package httpclient_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

// errRefusedHop is the error a test redirect check returns.
var errRefusedHop = errors.New("hop refused by the caller")

func TestNewWithNoOptionsReachesAServer(t *testing.T) {
	srv := newTLSServer(t, ok())
	client, err := httpclient.New(trust(t, srv))
	require.NoError(t, err)

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, httpclient.DefaultTimeout, client.Timeout)
}

func TestPinnedReachesOnlyItsHost(t *testing.T) {
	srv := newTLSServer(t, ok())
	client := pinnedTo(t, srv)

	status, err := fetch(t, client, srv.URL+"/v1/ping")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	// localhost resolves to the same server, but it is not the pinned host.
	_, err = fetch(t, client, "https://localhost:"+portOf(t, srv)+"/")
	require.ErrorIs(t, err, httpclient.ErrHostNotAllowed)
}

func TestPinnedMatchesHostWithoutRegardToCase(t *testing.T) {
	_, err := httpclient.Pinned("API.Example.COM.")
	require.NoError(t, err)
}

func TestPinnedIsHTTPSOnly(t *testing.T) {
	srv := newTLSServer(t, ok())
	client := pinnedTo(t, srv)

	_, err := fetch(t, client, "http://"+hostOf(t, srv)+":"+portOf(t, srv)+"/")
	require.ErrorIs(t, err, httpclient.ErrSchemeNotAllowed)
}

func TestPinnedReturnsRedirectsUnfollowed(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	client := pinnedTo(t, srv)

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, status)
}

func TestGuardedRefusesLoopbackAtDialTime(t *testing.T) {
	srv := newTLSServer(t, ok())
	client, err := httpclient.Guarded(trust(t, srv))
	require.NoError(t, err)

	for _, rawURL := range []string{srv.URL, "https://localhost:" + portOf(t, srv)} {
		_, err = fetch(t, client, rawURL)
		require.ErrorIsf(t, err, httpclient.ErrAddressBlocked, "%s must be refused", rawURL)
	}
}

func TestAddressGuardSeesTheDialedAddress(t *testing.T) {
	srv := newTLSServer(t, ok())

	var mu sync.Mutex
	var seen []netip.Addr
	guard := func(addr netip.Addr) bool {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, addr)

		return true
	}
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithAddressGuard(guard))
	require.NoError(t, err)

	_, err = fetch(t, client, srv.URL)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seen)
	assert.Equal(t, netip.MustParseAddr("127.0.0.1"), seen[0].Unmap())
}

func TestWithoutAddressGuardDialsAnyAddress(t *testing.T) {
	srv := newTLSServer(t, ok())
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithoutAddressGuard())
	require.NoError(t, err)

	status, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
}

// redirectServer redirects /hop/<n> to /hop/<n-1> until /hop/0, which answers
// 200.
func redirectServer(t *testing.T) string {
	t.Helper()
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n int
		if _, err := fmt.Sscanf(r.URL.Path, "/hop/%d", &n); err != nil || n == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
	}))

	return srv.URL
}

func TestGuardedFollowsRedirectsUpToTheLimit(t *testing.T) {
	srv := newTLSServer(t, ok())
	base := redirectServer(t)
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithAddressGuard(loopbackOnly))
	require.NoError(t, err)

	// redirectServer's certificate is httptest's shared one, which srv's trust
	// option also covers.
	status, err := fetch(t, client, base+"/hop/3")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	_, err = fetch(t, client, base+"/hop/4")
	require.ErrorIs(t, err, httpclient.ErrTooManyRedirects)
}

func TestRedirectCheckRunsOnEveryHop(t *testing.T) {
	srv := newTLSServer(t, ok())
	base := redirectServer(t)

	var hops []string
	check := func(req *http.Request, _ []*http.Request) error {
		hops = append(hops, req.URL.Path)
		if req.URL.Path == "/hop/1" {
			return errRefusedHop
		}

		return nil
	}
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithAddressGuard(loopbackOnly),
		httpclient.WithRedirectCheck(check))
	require.NoError(t, err)

	_, err = fetch(t, client, base+"/hop/3")
	require.ErrorIs(t, err, errRefusedHop)
	assert.Equal(t, []string{"/hop/2", "/hop/1"}, hops)
}

func TestHostAllowlistAppliesToEveryRedirectHop(t *testing.T) {
	target := newTLSServer(t, ok())
	targets := map[string]string{
		"/same-host":  target.URL,
		"/other-host": "https://localhost:" + portOf(t, target) + "/",
	}
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targets[r.URL.Path], http.StatusFound)
	}))
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithAddressGuard(loopbackOnly),
		httpclient.WithAllowedHosts("127.0.0.1"))
	require.NoError(t, err)

	// Another port on an allowed host is allowed.
	status, err := fetch(t, client, srv.URL+"/same-host")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	// localhost is the same machine, but not an allowed host.
	_, err = fetch(t, client, srv.URL+"/other-host")
	require.ErrorIs(t, err, httpclient.ErrHostNotAllowed)
}

func TestSchemeRulesApplyToEveryRedirectHop(t *testing.T) {
	plain := newPlainServer(t)
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain, http.StatusFound)
	}))
	client, err := httpclient.Guarded(trust(t, srv), httpclient.WithAddressGuard(loopbackOnly))
	require.NoError(t, err)

	_, err = fetch(t, client, srv.URL)
	require.ErrorIs(t, err, httpclient.ErrSchemeNotAllowed)
}

func TestAllowedSchemesCanAdmitPlainHTTP(t *testing.T) {
	plain := newPlainServer(t)
	client, err := httpclient.New(httpclient.WithAllowedSchemes("HTTP"))
	require.NoError(t, err)

	status, err := fetch(t, client, plain)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	_, err = fetch(t, client, strings.Replace(plain, "http://", "https://", 1))
	require.ErrorIs(t, err, httpclient.ErrSchemeNotAllowed)
}

func TestMaxBodySize(t *testing.T) {
	body := strings.Repeat("a", 11)
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			// Flushing first sends the body chunked, with no Content-Length.
			w.(http.Flusher).Flush()
		}
		_, _ = io.WriteString(w, body[:len(body)-len(r.URL.Query().Get("short"))])
	}))
	client := pinnedTo(t, srv, httpclient.WithMaxBodySize(10))

	t.Run("declared length over the limit fails before the body is read", func(t *testing.T) {
		_, err := fetch(t, client, srv.URL+"/")
		require.ErrorIs(t, err, httpclient.ErrResponseTooLarge)
	})

	t.Run("streamed body over the limit fails when read", func(t *testing.T) {
		_, err := readAll(t, client, srv.URL+"/chunked")
		require.ErrorIs(t, err, httpclient.ErrResponseTooLarge)
	})

	t.Run("body at the limit is read whole", func(t *testing.T) {
		got, err := readAll(t, client, srv.URL+"/chunked?short=x")
		require.NoError(t, err)
		assert.Equal(t, body[:10], got)
	})
}

// readAll sends a GET through client and reads the whole body.
func readAll(t *testing.T, client *http.Client, rawURL string) (string, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)

	return string(b), err
}

func TestMaxResponseHeaderBytes(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Large", strings.Repeat("h", 8<<10))
	}))
	client := pinnedTo(t, srv, httpclient.WithMaxResponseHeaderBytes(1<<10))

	_, err := fetch(t, client, srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "header")
}

func TestTimeouts(t *testing.T) {
	release := make(chan struct{})
	srv := newTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release) })

	tests := []struct {
		name string
		opt  httpclient.Option
	}{
		{name: "response header", opt: httpclient.WithResponseHeaderTimeout(20 * time.Millisecond)},
		{name: "overall", opt: httpclient.WithTimeout(20 * time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := pinnedTo(t, srv, tt.opt)
			_, err := fetch(t, client, srv.URL)

			var netErr net.Error
			require.ErrorAs(t, err, &netErr)
			assert.True(t, netErr.Timeout())
		})
	}
}

func TestPresetsIgnoreEnvironmentProxies(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	srv := newTLSServer(t, ok())

	status, err := fetch(t, pinnedTo(t, srv), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	rt, err := httpclient.NewTransport()
	require.NoError(t, err)
	assert.Nil(t, httpclient.BaseTransport(rt).Proxy)
}

func TestWithProxyFromEnvironmentSetsTheProxy(t *testing.T) {
	rt, err := httpclient.NewTransport(httpclient.WithProxyFromEnvironment())
	require.NoError(t, err)
	assert.NotNil(t, httpclient.BaseTransport(rt).Proxy)
}

func TestMinTLSVersion(t *testing.T) {
	srv := newTLS12Server(t)
	client := pinnedTo(t, srv, httpclient.WithMinTLSVersion(tls.VersionTLS13))

	_, err := fetch(t, client, srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol version")
}

func TestTLSConfigMinimumIsRaisedToTheClientMinimum(t *testing.T) {
	weak := &tls.Config{MinVersion: tls.VersionTLS10} //nolint:gosec // G402: the test proves the client raises it
	rt, err := httpclient.NewTransport(httpclient.WithTLSConfig(weak))
	require.NoError(t, err)

	got := httpclient.BaseTransport(rt).TLSClientConfig
	assert.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS10), weak.MinVersion, "the caller's config is not changed")
}

func TestStaticHeaders(t *testing.T) {
	got := make(chan http.Header, 2)
	srv := newTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
	}))
	client := pinnedTo(t, srv, httpclient.WithUserAgent("example-agent/1.0"),
		httpclient.WithHeader("X-Client", "static"))

	_, err := fetch(t, client, srv.URL)
	require.NoError(t, err)
	h := <-got
	assert.Equal(t, "example-agent/1.0", h.Get("User-Agent"))
	assert.Equal(t, "static", h.Get("X-Client"))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "own-agent")
	req.Header.Set("X-Client", "own")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	h = <-got
	assert.Equal(t, "own-agent", h.Get("User-Agent"), "the request's own header wins")
	assert.Equal(t, "own", h.Get("X-Client"))
}

func TestStaticHeadersDoNotChangeTheCallersRequest(t *testing.T) {
	srv := newTLSServer(t, ok())
	client := pinnedTo(t, srv, httpclient.WithHeader("X-Client", "static"))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Empty(t, req.Header.Get("X-Client"))
}

func TestConcurrentRequestsShareOneClient(t *testing.T) {
	srv := newTLSServer(t, ok())

	var mu sync.Mutex
	events := 0
	client := pinnedTo(t, srv, httpclient.WithObserver(func(httpclient.Event) {
		mu.Lock()
		defer mu.Unlock()
		events++
	}), httpclient.WithRetry(httpclient.RetryPolicy{}))

	type result struct {
		status int
		err    error
	}
	results := make(chan result, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			status, err := fetch(t, client, srv.URL)
			results <- result{status: status, err: err}
		})
	}
	wg.Wait()
	close(results)
	client.CloseIdleConnections()

	for r := range results {
		require.NoError(t, r.err)
		assert.Equal(t, http.StatusOK, r.status)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 16, events)
}

func TestTransportClosesTheBodyOfARefusedRequest(t *testing.T) {
	rt, err := httpclient.NewTransport(httpclient.WithAllowedHosts("api.example.com"))
	require.NoError(t, err)

	body := &closeRecorder{Reader: strings.NewReader("payload")}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://other.example.com/", body)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.ErrorIs(t, err, httpclient.ErrHostNotAllowed)
	assert.True(t, body.closed)
}

// closeRecorder is a request body that records whether it was closed.
type closeRecorder struct {
	io.Reader

	closed bool
}

// Close records the close.
func (c *closeRecorder) Close() error {
	c.closed = true

	return nil
}

func TestTransportAddsStaticHeadersToARequestWithoutAHeaderMap(t *testing.T) {
	got := make(chan string, 1)
	srv := newTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Client")
	}))
	rt, err := httpclient.NewTransport(trust(t, srv), httpclient.WithHeader("X-Client", "static"))
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header = nil
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "static", <-got)
}

func TestPinnedRefusesAnOverlongHost(t *testing.T) {
	_, err := httpclient.Pinned(strings.Repeat("a.", 127) + "com")
	require.ErrorIs(t, err, httpclient.ErrInvalidConfig)
}

func TestMaxBodySizeNearTheIntegerLimit(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "body")
	}))
	client := pinnedTo(t, srv, httpclient.WithMaxBodySize(math.MaxInt64))

	got, err := readAll(t, client, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "body", got)
}

// BenchmarkPinned measures building a Pinned client with a few options.
func BenchmarkPinned(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		if _, err := httpclient.Pinned("api.example.com", httpclient.WithUserAgent("bench/1.0")); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPinnedGet measures a GET through a Pinned client, with an observer,
// to a local TLS server over a kept-alive connection: the cost the enforcing
// transport adds on top of net/http.
func BenchmarkPinnedGet(b *testing.B) {
	srv := httptest.NewTLSServer(ok())
	b.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	client, err := httpclient.Pinned("127.0.0.1",
		httpclient.WithTLSConfig(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}),
		httpclient.WithAddressGuard(loopbackOnly),
		httpclient.WithObserver(func(httpclient.Event) {}),
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for range b.N {
		req, _ := http.NewRequestWithContext(b.Context(), http.MethodGet, srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
