package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	entry := parseLogEntry(t, []byte(output))
	assert.Equal(t, "panic", entry.Type)
	assert.Equal(t, "err-panic-id", entry.RequestID)
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

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	var resp httputil.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp),
		"response body is not a valid ErrorResponse: %s", rr.Body.String())

	assert.Equal(t, constants.ErrorCodeInternalError, resp.Code)
	assert.Equal(t, constants.ErrorMessageInternalError, resp.Error)
	assert.Equal(t, "body-req-id", resp.RequestID)
}
