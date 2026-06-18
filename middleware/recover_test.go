package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrz1836/go-foundation/constants"
	"github.com/mrz1836/go-foundation/httputil"
	"github.com/mrz1836/go-foundation/middleware"
)

// TestRecoverMiddleware_PanicWithError confirms a panic carrying an error value
// (rather than a string) is recovered, logged with the error text, and answered
// with a 500.
func TestRecoverMiddleware_PanicWithError(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(context.DeadlineExceeded) // a real error value, not a string
	})

	mw := middleware.RecoverMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "err-panic-id")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}

	entry := parseLogEntry(t, []byte(output))
	if entry.Type != "panic" {
		t.Errorf("type = %q, want 'panic'", entry.Type)
	}

	if entry.RequestID != "err-panic-id" {
		t.Errorf("request_id = %q, want 'err-panic-id'", entry.RequestID)
	}
}

// TestRecoverMiddleware_ResponseBody verifies the 500 response body is the
// standard structured error envelope with the internal-error code and the
// propagated request ID.
func TestRecoverMiddleware_ResponseBody(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	mw := middleware.RecoverMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "body-req-id")

	rr := httptest.NewRecorder()

	captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}

	var resp httputil.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response body is not a valid ErrorResponse: %v\nbody: %s", err, rr.Body.String())
	}

	if resp.Code != constants.ErrorCodeInternalError {
		t.Errorf("code = %q, want %q", resp.Code, constants.ErrorCodeInternalError)
	}

	if resp.Error != constants.ErrorMessageInternalError {
		t.Errorf("error = %q, want %q", resp.Error, constants.ErrorMessageInternalError)
	}

	if resp.RequestID != "body-req-id" {
		t.Errorf("request_id = %q, want 'body-req-id'", resp.RequestID)
	}
}
