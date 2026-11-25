package httpclient_test

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

func TestNewRefusesInvalidConfiguration(t *testing.T) {
	noop := func(*http.Request, []*http.Request) error { return nil }

	tests := []struct {
		name string
		opts []httpclient.Option
	}{
		{name: "negative overall timeout", opts: opts(httpclient.WithTimeout(-time.Second))},
		{name: "negative dial timeout", opts: opts(httpclient.WithDialTimeout(-time.Second))},
		{name: "negative TLS timeout", opts: opts(httpclient.WithTLSHandshakeTimeout(-time.Second))},
		{name: "negative header timeout", opts: opts(httpclient.WithResponseHeaderTimeout(-time.Second))},
		{name: "negative idle timeout", opts: opts(httpclient.WithIdleConnTimeout(-time.Second))},
		{name: "negative body limit", opts: opts(httpclient.WithMaxBodySize(-1))},
		{name: "negative header limit", opts: opts(httpclient.WithMaxResponseHeaderBytes(-1))},
		{name: "negative redirects", opts: opts(httpclient.WithMaxRedirects(-1))},
		{
			name: "redirect check that can never run",
			opts: opts(httpclient.WithNoRedirects(), httpclient.WithRedirectCheck(noop)),
		},
		{name: "no schemes", opts: opts(httpclient.WithAllowedSchemes())},
		{name: "unknown scheme", opts: opts(httpclient.WithAllowedSchemes("ftp"))},
		{name: "empty host allowlist", opts: opts(httpclient.WithAllowedHosts())},
		{name: "host with a scheme", opts: opts(httpclient.WithAllowedHosts("https://api.example.com"))},
		{name: "host with a port", opts: opts(httpclient.WithAllowedHosts("api.example.com:443"))},
		{name: "host with a path", opts: opts(httpclient.WithAllowedHosts("api.example.com/v1"))},
		{name: "host with userinfo", opts: opts(httpclient.WithAllowedHosts("user@api.example.com"))},
		{name: "wildcard host", opts: opts(httpclient.WithAllowedHosts("*.example.com"))},
		{name: "empty label", opts: opts(httpclient.WithAllowedHosts("api..example.com"))},
		{name: "label starting with a hyphen", opts: opts(httpclient.WithAllowedHosts("-api.example.com"))},
		{name: "empty host", opts: opts(httpclient.WithAllowedHosts(""))},
		{name: "zoned IP", opts: opts(httpclient.WithAllowedHosts("fe80::1%eth0"))},
		{name: "nil address guard", opts: opts(httpclient.WithAddressGuard(nil))},
		{
			name: "address guard with an environment proxy",
			opts: opts(httpclient.WithAddressGuard(httpclient.IsPublicAddress), httpclient.WithProxyFromEnvironment()),
		},
		{name: "unknown TLS version", opts: opts(httpclient.WithMinTLSVersion(0x0200))},
		{
			name: "TLS maximum below the minimum",
			opts: opts(httpclient.WithMinTLSVersion(tls.VersionTLS13),
				//nolint:gosec // G402: the test proves a low maximum is refused
				httpclient.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12})),
		},
		{name: "invalid user agent", opts: opts(httpclient.WithUserAgent("agent\r\nX-Injected: 1"))},
		{name: "invalid header name", opts: opts(httpclient.WithHeader("Bad Header", "v"))},
		{name: "invalid header value", opts: opts(httpclient.WithHeader("X-Ok", "v\n"))},
		{name: "negative retry attempts", opts: opts(httpclient.WithRetry(httpclient.RetryPolicy{MaxAttempts: -1}))},
		{
			name: "retry max delay below base delay",
			opts: opts(httpclient.WithRetry(httpclient.RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Millisecond})),
		},
		{name: "invalid retry method", opts: opts(httpclient.WithRetry(httpclient.RetryPolicy{Methods: []string{"GE T"}}))},
		{name: "invalid retry status", opts: opts(httpclient.WithRetry(httpclient.RetryPolicy{Statuses: []int{99}}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := httpclient.New(tt.opts...)
			require.ErrorIs(t, err, httpclient.ErrInvalidConfig)
			assert.Nil(t, client)

			rt, err := httpclient.NewTransport(tt.opts...)
			require.ErrorIs(t, err, httpclient.ErrInvalidConfig)
			assert.Nil(t, rt)
		})
	}
}

// opts collects options for a table row.
func opts(o ...httpclient.Option) []httpclient.Option {
	return o
}

func TestInvalidConfigErrorDoesNotQuoteAHeaderValue(t *testing.T) {
	_, err := httpclient.New(httpclient.WithHeader("X-Api-Key", fakeSecret+"\n"))
	require.ErrorIs(t, err, httpclient.ErrInvalidConfig)
	assert.NotContains(t, err.Error(), fakeSecret)
}

func TestPinnedRefusesAnInvalidHost(t *testing.T) {
	for _, host := range []string{"", "https://api.example.com", "api.example.com:443", "*.example.com"} {
		client, err := httpclient.Pinned(host)
		require.ErrorIsf(t, err, httpclient.ErrInvalidConfig, "host %q", host)
		assert.Nil(t, client)
	}
}

func TestPinnedRefusesAChangedAllowlist(t *testing.T) {
	_, err := httpclient.Pinned("api.example.com", httpclient.WithAllowedHosts("other.example.com"))
	require.ErrorIs(t, err, httpclient.ErrInvalidConfig)

	_, err = httpclient.Pinned("api.example.com", httpclient.WithAllowedHosts("API.example.com"))
	require.NoError(t, err, "restating the pinned host is not a change")
}

func TestPresetsRefuseAProxyUnlessTheGuardIsRemoved(t *testing.T) {
	_, err := httpclient.Guarded(httpclient.WithProxyFromEnvironment())
	require.ErrorIs(t, err, httpclient.ErrInvalidConfig)

	_, err = httpclient.Pinned("api.example.com", httpclient.WithProxyFromEnvironment())
	require.ErrorIs(t, err, httpclient.ErrInvalidConfig)

	_, err = httpclient.Guarded(httpclient.WithProxyFromEnvironment(), httpclient.WithoutAddressGuard())
	require.NoError(t, err)
}

func TestPresetDefaults(t *testing.T) {
	pinned, err := httpclient.Pinned("api.example.com")
	require.NoError(t, err)
	guarded, err := httpclient.Guarded()
	require.NoError(t, err)

	for _, client := range []*http.Client{pinned, guarded} {
		assert.Equal(t, httpclient.GuardedTimeout, client.Timeout)

		base := httpclient.BaseTransport(client.Transport)
		require.NotNil(t, base)
		assert.Nil(t, base.Proxy)
		assert.Equal(t, httpclient.GuardedTLSHandshakeTimeout, base.TLSHandshakeTimeout)
		assert.Equal(t, httpclient.GuardedResponseHeaderTimeout, base.ResponseHeaderTimeout)
		assert.Equal(t, int64(httpclient.GuardedMaxResponseHeaderBytes), base.MaxResponseHeaderBytes)
		assert.Equal(t, uint16(tls.VersionTLS12), base.TLSClientConfig.MinVersion)
	}
}

func TestNilOptionIsIgnored(t *testing.T) {
	_, err := httpclient.New(nil, httpclient.WithTimeout(time.Second))
	require.NoError(t, err)
}
