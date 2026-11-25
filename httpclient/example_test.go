package httpclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/mrz1836/go-foundation/httpclient"
)

// ExamplePinned builds a client for one known API. It reaches only that host,
// over HTTPS, without a proxy, and never dials a non-public address.
func ExamplePinned() {
	client, err := httpclient.Pinned("api.example.com",
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithUserAgent("example-app/1.0"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Any other host is refused before a connection is made.
	err = httpclient.GetJSON(context.Background(), client, "https://other.example.com/", nil, nil)
	fmt.Println(errors.Is(err, httpclient.ErrHostNotAllowed))
	// Output: true
}

// ExampleGuarded builds a client for URLs that are not known in advance, and
// re-checks each redirect with the caller's own rule.
func ExampleGuarded() {
	client, err := httpclient.Guarded(
		httpclient.WithRedirectCheck(func(req *http.Request, _ []*http.Request) error {
			if req.URL.Path == "/forbidden" {
				// Stop here and return the redirect response itself.
				return http.ErrUseLastResponse
			}

			return nil
		}),
		httpclient.WithRetry(httpclient.RetryPolicy{MaxAttempts: 3}),
		httpclient.WithObserver(func(e httpclient.Event) {
			// Safe to log: no headers, body, or query string.
			_ = e
		}),
	)
	fmt.Println(client != nil, err)
	// Output: true <nil>
}

// ExampleNew shows that New refuses a configuration that contradicts itself.
func ExampleNew() {
	_, err := httpclient.New(
		httpclient.WithAddressGuard(httpclient.IsPublicAddress),
		httpclient.WithProxyFromEnvironment(),
	)
	fmt.Println(errors.Is(err, httpclient.ErrInvalidConfig))
	// Output: true
}

// ExampleIsPublicAddress shows the default address guard.
func ExampleIsPublicAddress() {
	for _, s := range []string{"93.184.216.34", "10.0.0.5", "169.254.169.254", "::ffff:127.0.0.1"} {
		fmt.Println(s, httpclient.IsPublicAddress(netip.MustParseAddr(s)))
	}
	// Output:
	// 93.184.216.34 true
	// 10.0.0.5 false
	// 169.254.169.254 false
	// ::ffff:127.0.0.1 false
}

// ExampleGetJSON decodes a JSON response. A status outside 2xx returns a
// *StatusError, which never holds the response body.
func ExampleGetJSON() {
	client, err := httpclient.Pinned("api.example.com")
	if err != nil {
		fmt.Println(err)
		return
	}

	var out struct {
		Status string `json:"status"`
	}
	header := http.Header{"Authorization": {"Bearer <token>"}}
	err = httpclient.GetJSON(context.Background(), client, "https://api.example.com/v1/status", header, &out)

	var statusErr *httpclient.StatusError
	if errors.As(err, &statusErr) {
		fmt.Println("status", statusErr.StatusCode)
	}
}
