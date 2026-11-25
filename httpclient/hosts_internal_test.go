package httpclient

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzHostAllowlist checks that normalizing an allowlist entry is idempotent,
// that no URL panics the scheme and host check, and that a URL the check
// passes names exactly the allowed host.
func FuzzHostAllowlist(f *testing.F) {
	seeds := [][2]string{
		{"api.example.com", "https://API.example.com./v1?x=1"},
		{"api.example.com", "https://api.example.com.evil.test/"},
		{"api.example.com", "https://user@api.example.com:8443/"},
		{"127.0.0.1", "https://127.0.0.1:443/"},
		{"2001:db8::1", "https://[2001:DB8:0::1]/"},
		{"xn--bcher-kva.example", "https://bücher.example/"},
		{"a_b.example.com", "http://a_b.example.com/%2e%2e"},
		{"api.example.com", "https://api%2eexample.com/"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, host, rawURL string) {
		normalized, err := normalizeHost(host)
		if err != nil {
			return
		}
		again, err := normalizeHost(normalized)
		require.NoError(t, err)
		require.Equal(t, normalized, again)

		u, err := url.Parse(rawURL)
		if err != nil {
			return
		}
		tr := &transport{
			schemes: []string{schemeHTTP, schemeHTTPS},
			hosts:   map[string]struct{}{normalized: {}},
		}
		if tr.check(u) == nil {
			require.Equal(t, normalized, requestHost(u))
		}
	})
}
