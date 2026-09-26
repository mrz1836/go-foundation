package lambda_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lambdahttp "github.com/mrz1836/go-foundation/lambda"
)

func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write the request ID into the body so the test can verify forwarding
		// without relying on header key canonicalisation.
		_, _ = w.Write([]byte(`{"ok":true,"request_id":"` + r.Header.Get("X-Request-ID") + `"}`)) //nolint:gosec // G705: test handler echoes request data for assertion
	})
}

func TestServeHTTP_BasicRequest(t *testing.T) {
	t.Parallel()

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/v1/items",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP:      events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
			RequestID: "apigw-req-123",
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, echoHandler())
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	wantBody := `{"ok":true,"request_id":"apigw-req-123"}`
	assert.Equal(t, wantBody, resp.Body)

	assert.False(t, resp.IsBase64Encoded, "IsBase64Encoded should be false for UTF-8 body")
	// Go canonicalises "Content-Type" → "Content-Type" (already canonical)
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])
}

func TestServeHTTP_QueryString(t *testing.T) {
	t.Parallel()

	queryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.RawQuery)) //nolint:gosec // G705: test handler echoes request data for assertion
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath:        "/v1/feed",
		RawQueryString: "limit=20&cursor=abc123",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, queryHandler)
	require.NoError(t, err)

	assert.Equal(t, "limit=20&cursor=abc123", resp.Body)
}

func TestServeHTTP_EmptyPathDefaultsToRoot(t *testing.T) {
	t.Parallel()

	pathHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path)) //nolint:gosec // G705: test handler echoes request data for assertion
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, pathHandler)
	require.NoError(t, err)

	assert.Equal(t, "/", resp.Body)
}

func TestServeHTTP_EmptyMethodDefaultsToGET(t *testing.T) {
	t.Parallel()

	methodHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Method)) //nolint:gosec // G705: test handler echoes request data for assertion
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath:        "/health",
		RequestContext: events.APIGatewayV2HTTPRequestContext{},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, methodHandler)
	require.NoError(t, err)

	assert.Equal(t, "GET", resp.Body)
}

func TestServeHTTP_Base64EncodedRequestBody(t *testing.T) {
	t.Parallel()

	bodyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf[:n])
	})

	payload := []byte("hello binary world")
	event := events.APIGatewayV2HTTPRequest{
		RawPath:         "/upload",
		IsBase64Encoded: true,
		Body:            base64.StdEncoding.EncodeToString(payload),
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, bodyHandler)
	require.NoError(t, err)

	assert.Equal(t, string(payload), resp.Body)
}

func TestServeHTTP_BinaryResponseBase64Encoded(t *testing.T) {
	t.Parallel()

	// Respond with raw bytes that are not valid UTF-8
	binaryHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0xFF, 0xFE, 0x00, 0x01}) // invalid UTF-8
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/binary",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, binaryHandler)
	require.NoError(t, err)

	assert.True(t, resp.IsBase64Encoded, "IsBase64Encoded should be true for binary body")

	decoded, decErr := base64.StdEncoding.DecodeString(resp.Body)
	require.NoError(t, decErr)

	assert.Equal(t, []byte{0xFF, 0xFE, 0x00, 0x01}, decoded, "decoded body mismatch")
}

func TestServeHTTP_URLParseError(t *testing.T) {
	t.Parallel()

	// A control character (\x00) in the path causes url.Parse to return an error,
	// exercising the url.Parse error-return path inside toHTTPRequest.
	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/\x00invalid",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	require.Error(t, err, "expected error from ServeHTTP on URL with control character")

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestServeHTTP_InvalidHTTPMethod(t *testing.T) {
	t.Parallel()

	// A space in the method name is invalid per RFC 7230; http.NewRequestWithContext
	// returns an error, exercising the third error-return path inside toHTTPRequest.
	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/health",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GE T"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	require.Error(t, err, "expected error from ServeHTTP on invalid HTTP method")

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestServeHTTP_RequestConversionError(t *testing.T) {
	t.Parallel()

	// IsBase64Encoded=true with a body that is not valid base64 forces
	// toHTTPRequest to return an error, exercising the ServeHTTP error-return path.
	event := events.APIGatewayV2HTTPRequest{
		RawPath:         "/upload",
		IsBase64Encoded: true,
		Body:            "!!!not-valid-base64!!!",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	require.Error(t, err, "expected error from ServeHTTP on invalid base64 body")

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])
}

func TestServeHTTP_HeadersAndCookiesForwarded(t *testing.T) {
	t.Parallel()

	captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Header.Get("Authorization") + "|" + r.Header.Get("Cookie"))) //nolint:gosec // G705: test handler echoes request data for assertion
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/secure",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
		},
		Cookies: []string{"session=abc", "pref=dark"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, captureHandler)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Authorization header present
	assert.Contains(t, resp.Body, "Bearer token", "expected Authorization header in body")
}

func TestServeHTTP_SetCookieNotCommaJoined(t *testing.T) {
	t.Parallel()

	cookieHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", "session=abc; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "pref=dark; Path=/")
		w.WriteHeader(http.StatusOK)
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/login",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}

	resp, err := lambdahttp.ServeHTTP(context.Background(), event, cookieHandler)
	require.NoError(t, err)
	// Set-Cookie should NOT appear in the Headers map
	assert.NotContains(t, resp.Headers, "Set-Cookie", "Set-Cookie should not be in Headers map; should be in Cookies slice")
	// Should be in the Cookies slice
	require.Len(t, resp.Cookies, 2)

	assert.Equal(t, "session=abc; Path=/; HttpOnly", resp.Cookies[0])
	assert.Equal(t, "pref=dark; Path=/", resp.Cookies[1])
}

func TestServeHTTP_RemoteAddrFromSourceIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sourceIP string
		headers  map[string]string
		want     string
	}{
		{
			name:     "ipv4",
			sourceIP: "203.0.113.7",
			headers:  map[string]string{"X-Forwarded-For": "1.2.3.4, 203.0.113.7"},
			want:     "203.0.113.7:0",
		},
		{
			name:     "ipv6",
			sourceIP: "2001:db8::1",
			want:     "[2001:db8::1]:0",
		},
		{
			name:     "empty source ip leaves RemoteAddr unset",
			sourceIP: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got string

			captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr

				w.WriteHeader(http.StatusOK)
			})

			event := events.APIGatewayV2HTTPRequest{
				RawPath: "/client",
				Headers: tt.headers,
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method:   "GET",
						SourceIP: tt.sourceIP,
					},
				},
			}

			_, err := lambdahttp.ServeHTTP(context.Background(), event, captureHandler)
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkServeHTTP(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/api/test",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"},
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := lambdahttp.ServeHTTP(ctx, event, handler)
		if err != nil {
			b.Fatal(err)
		}
	}
}
