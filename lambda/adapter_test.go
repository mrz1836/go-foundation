package lambda_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	wantBody := `{"ok":true,"request_id":"apigw-req-123"}`
	if resp.Body != wantBody {
		t.Errorf("Body = %q, want %q", resp.Body, wantBody)
	}

	if resp.IsBase64Encoded {
		t.Error("IsBase64Encoded should be false for UTF-8 body")
	}
	// Go canonicalises "Content-Type" → "Content-Type" (already canonical)
	if resp.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header = %q", resp.Headers["Content-Type"])
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.Body != "limit=20&cursor=abc123" {
		t.Errorf("Body = %q, want 'limit=20&cursor=abc123'", resp.Body)
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.Body != "/" {
		t.Errorf("Body = %q, want '/'", resp.Body)
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.Body != "GET" {
		t.Errorf("Body = %q, want 'GET'", resp.Body)
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.Body != string(payload) {
		t.Errorf("Body = %q, want %q", resp.Body, string(payload))
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if !resp.IsBase64Encoded {
		t.Error("IsBase64Encoded should be true for binary body")
	}

	decoded, decErr := base64.StdEncoding.DecodeString(resp.Body)
	if decErr != nil {
		t.Fatalf("base64 decode: %v", decErr)
	}

	if string(decoded) != string([]byte{0xFF, 0xFE, 0x00, 0x01}) {
		t.Errorf("decoded body mismatch")
	}
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
	if err == nil {
		t.Fatal("expected error from ServeHTTP on URL with control character, got nil")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", resp.StatusCode)
	}
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
	if err == nil {
		t.Fatal("expected error from ServeHTTP on invalid HTTP method, got nil")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", resp.StatusCode)
	}
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
	if err == nil {
		t.Fatal("expected error from ServeHTTP on invalid base64 body, got nil")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", resp.StatusCode)
	}

	if resp.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", resp.Headers["Content-Type"])
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	// Authorization header present
	if len(resp.Body) == 0 || resp.Body[:12] != "Bearer token" {
		t.Errorf("expected Authorization header in body, got %q", resp.Body)
	}
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
	if err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	// Set-Cookie should NOT appear in the Headers map
	if _, ok := resp.Headers["Set-Cookie"]; ok {
		t.Error("Set-Cookie should not be in Headers map; should be in Cookies slice")
	}
	// Should be in the Cookies slice
	if len(resp.Cookies) != 2 {
		t.Fatalf("Cookies length = %d, want 2", len(resp.Cookies))
	}

	if resp.Cookies[0] != "session=abc; Path=/; HttpOnly" {
		t.Errorf("Cookies[0] = %q", resp.Cookies[0])
	}

	if resp.Cookies[1] != "pref=dark; Path=/" {
		t.Errorf("Cookies[1] = %q", resp.Cookies[1])
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
