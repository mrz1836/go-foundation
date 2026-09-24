package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/middleware"
)

// logEntry mirrors the JSON shape of both requestLog and responseLog so tests
// can assert on any field without depending on unexported struct types.
type logEntry struct {
	Timestamp    string `json:"time"`
	Level        string `json:"level"`
	Type         string `json:"type"`
	RequestID    string `json:"request_id"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	SourceIP     string `json:"source_ip"`
	UserAgent    string `json:"user_agent,omitempty"`
	Status       int    `json:"status,omitempty"`
	Latency      int64  `json:"latency"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// captureLogOutput redirects the standard logger to a buffer for the duration
// of f, then restores the original output.
func captureLogOutput(f func()) string {
	var buf bytes.Buffer

	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	oldSlog := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(oldSlog)

	f()

	return buf.String()
}

// parseLogEntry finds the first '{' in line, unmarshals from there, and fails
// the test immediately if anything goes wrong.
func parseLogEntry(t *testing.T, line []byte) logEntry {
	t.Helper()

	jsonStart := bytes.Index(line, []byte("{"))
	require.NotEqual(t, -1, jsonStart, "no JSON object found in log line: %q", string(line))

	var entry logEntry
	require.NoError(t, json.Unmarshal(line[jsonStart:], &entry), "failed to parse log JSON; log line: %s", string(line))

	return entry
}

func assertRequestEntry(t *testing.T, entry logEntry, wantRequestID, wantUserAgent, wantLevel string) {
	t.Helper()

	assert.Equal(t, "request", entry.Type)
	assert.Equal(t, wantRequestID, entry.RequestID)
	assert.Equal(t, wantUserAgent, entry.UserAgent)
	assert.Equal(t, wantLevel, entry.Level)
}

func assertResponseEntry(t *testing.T, entry logEntry, wantStatus int, wantLevel string) {
	t.Helper()

	assert.Equal(t, "response", entry.Type)
	assert.Equal(t, wantStatus, entry.Status)
	assert.Equal(t, wantLevel, entry.Level)
	assert.GreaterOrEqual(t, entry.Latency, int64(0))
}

func TestLoggingMiddleware_RequestResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	})

	mw := middleware.LoggingMiddleware(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Request-ID", "test-123")
	req.Header.Set("User-Agent", "curl/7.88.0")

	rr := httptest.NewRecorder()
	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	assert.Equal(t, http.StatusOK, rr.Code)

	lines := bytes.Split([]byte(output), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2, "output: %s", output)

	assertRequestEntry(t, parseLogEntry(t, lines[0]), "test-123", "curl/7.88.0", "INFO")
	assertResponseEntry(t, parseLogEntry(t, lines[1]), http.StatusOK, "INFO")
}

func TestLoggingMiddleware_ClientError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"resource not found"}`))
	})

	mw := middleware.LoggingMiddleware(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/missing", nil)
	req.Header.Set("X-Request-ID", "test-456")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2)

	respEntry := parseLogEntry(t, lines[1])
	assert.Equal(t, "WARN", respEntry.Level)
	assert.Equal(t, "resource not found", respEntry.ErrorMessage)
}

func TestLoggingMiddleware_ServerError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"database connection timeout"}`))
	})

	mw := middleware.LoggingMiddleware(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/users", nil)
	req.Header.Set("X-Request-ID", "test-error")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))
	respEntry := parseLogEntry(t, lines[1])

	assert.Equal(t, "ERROR", respEntry.Level)
	assert.Equal(t, "database connection timeout", respEntry.ErrorMessage)
}

func TestLoggingMiddleware_LogInjectionPrevention(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.LoggingMiddleware(handler)

	maliciousPath := `/foo","level":"ERROR","injected":"true`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, maliciousPath, nil)
	req.Header.Set("X-Request-ID", "test-injection")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))
	require.NotEmpty(t, lines)

	entry := parseLogEntry(t, lines[0])
	assert.Equal(t, maliciousPath, entry.Path)

	// Confirm no injected top-level key appeared
	var raw map[string]any

	jsonStart := bytes.Index(lines[0], []byte("{"))
	require.NoError(t, json.Unmarshal(lines[0][jsonStart:], &raw), "log line is not valid JSON")

	assert.NotContains(t, raw, "injected", "log injection succeeded: 'injected' key found at top level")
}

func TestLoggingMiddleware_AmazonRequestIDFallback(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.LoggingMiddleware(handler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	req.Header.Set("X-Amzn-Request-Id", "amzn-req-xyz")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })
	lines := bytes.Split([]byte(output), []byte("\n"))

	entry := parseLogEntry(t, lines[0])
	assert.Equal(t, "amzn-req-xyz", entry.RequestID)
}

func TestLoggingMiddleware_EmptyBodyWith4xx(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		// No body written — extractErrorMessage must return ""
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Empty(t, respEntry.ErrorMessage, "error_message should be empty for empty body")
}

func TestLoggingMiddleware_NonJSONBodyWith4xx(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("plain text error"))
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/bad", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Empty(t, respEntry.ErrorMessage, "error_message should be empty for non-JSON body")
}

func TestLoggingMiddleware_ErrorMessageKeyExtraction(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error_message":"validation failed"}`))
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/validate", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Equal(t, "validation failed", respEntry.ErrorMessage)
}

func TestLoggingMiddleware_NonStringErrorValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":42}`)) // numeric value, not a string
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/numeric", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Empty(t, respEntry.ErrorMessage, "error_message should be empty for non-string error value")
}

func TestLoggingMiddleware_NoMatchingErrorKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"custom":"unrecognized"}`))
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/custom", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Empty(t, respEntry.ErrorMessage, "error_message should be empty when no recognized key present")
}

func TestLoggingMiddleware_RequestIDInContext(t *testing.T) {
	var capturedID string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.RequestIDFromContext(r.Context())

		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ctx", nil)
	req.Header.Set("X-Request-ID", "ctx-req-id")

	rr := httptest.NewRecorder()

	captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	assert.Equal(t, "ctx-req-id", capturedID)
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	t.Parallel()

	id := middleware.RequestIDFromContext(context.Background())
	assert.Empty(t, id, "RequestIDFromContext on empty context should be empty")
}

func TestLoggingMiddleware_DoubleWriteHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.WriteHeader(http.StatusOK) // second call should be ignored
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/double", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	// The actual response should be 400 (first WriteHeader wins)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// The logged status should also be 400
	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Equal(t, http.StatusBadRequest, respEntry.Status, "logged status should be 400")
}

func TestRecoverMiddleware_AmazonRequestIDFallback(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("amzn fallback panic")
	})

	mw := middleware.RecoverMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	// Only the Amazon header is set — X-Request-ID is absent.
	req.Header.Set("X-Amzn-Request-Id", "amzn-panic-id")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var panicEntry struct {
		RequestID string `json:"request_id"`
	}

	lines := bytes.Split([]byte(output), []byte("\n"))

	jsonStart := bytes.Index(lines[0], []byte("{"))
	require.NotEqual(t, -1, jsonStart, "no JSON in panic log: %q", string(lines[0]))

	require.NoError(t, json.Unmarshal(lines[0][jsonStart:], &panicEntry), "unmarshal panic log")

	assert.Equal(t, "amzn-panic-id", panicEntry.RequestID)
}

func TestRecoverMiddleware_NoPanic(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mw := middleware.RecoverMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRecoverMiddleware_PanicReturns500(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("something went terribly wrong")
	})

	mw := middleware.RecoverMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "panic-req-id")

	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	// Confirm structured panic log was written
	var panicEntry struct {
		Level     string `json:"level"`
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
		Stack     string `json:"stack"`
	}

	lines := bytes.Split([]byte(output), []byte("\n"))

	jsonStart := bytes.Index(lines[0], []byte("{"))
	require.NotEqual(t, -1, jsonStart, "no JSON in panic log: %q", string(lines[0]))

	require.NoError(t, json.Unmarshal(lines[0][jsonStart:], &panicEntry), "unmarshal panic log")

	assert.Equal(t, "ERROR", panicEntry.Level)
	assert.Equal(t, "panic", panicEntry.Type)
	assert.Equal(t, "panic-req-id", panicEntry.RequestID)
	assert.NotEmpty(t, panicEntry.Stack, "stack should be non-empty")
}

func TestLoggingMiddleware_Unwrap(t *testing.T) {
	// http.NewResponseController.Flush walks the Unwrap chain to reach the
	// underlying http.Flusher (httptest.ResponseRecorder implements it). A nil
	// error proves responseWriter.Unwrap returned the inner writer.
	var flushErr error

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))

		flushErr = http.NewResponseController(w).Flush()
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/flush", nil)
	rr := httptest.NewRecorder()

	captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	require.NoError(t, flushErr, "Flush via ResponseController should be nil (Unwrap must expose inner writer)")
	assert.True(t, rr.Flushed, "recorder was not flushed; Unwrap did not reach the inner http.Flusher")
}

func TestLoggingMiddleware_ErrorBodyTruncated(t *testing.T) {
	const maxErrorBodySize = 4096

	// A single Write larger than maxErrorBodySize forces the truncation branch
	// (len(b) > remaining) in responseWriter.Write.
	bigBody := `{"error":"` + strings.Repeat("x", maxErrorBodySize*2) + `"}`

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(bigBody))
	})

	mw := middleware.LoggingMiddleware(handler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/big", nil)
	rr := httptest.NewRecorder()

	output := captureLogOutput(func() { mw.ServeHTTP(rr, req) })

	// The full body is still written downstream to the client.
	assert.Equal(t, len(bigBody), rr.Body.Len(), "downstream write must be intact")

	// The captured (truncated) body is not valid JSON, so extractErrorMessage
	// returns "" — confirming only the first 4096 bytes were retained.
	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	assert.Equal(t, http.StatusInternalServerError, respEntry.Status)
	assert.Empty(t, respEntry.ErrorMessage, "error_message should be empty (truncated body is not valid JSON)")
}

func BenchmarkLoggingMiddleware_LargeResponse200(b *testing.B) {
	largeBody := make([]byte, 10*1024*1024) // 10 MB
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeBody)
	})

	mw := middleware.LoggingMiddleware(handler)

	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	oldSlog := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(oldSlog)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/bench", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}
}

func BenchmarkLoggingMiddleware_RequestPath(b *testing.B) {
	// A no-op 200 handler isolates the per-request overhead: request-ID
	// extraction, context injection, the request-start log, and the response
	// log. Headers are pre-populated so the request-side fields are realistic.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.LoggingMiddleware(handler)

	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	oldSlog := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(oldSlog)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/resource", nil)
		req.Header.Set("X-Request-ID", "bench-req-id")
		req.Header.Set("User-Agent", "bench-agent/1.0")

		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}
}

func BenchmarkLoggingMiddleware_ErrorResponse(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	})

	mw := middleware.LoggingMiddleware(handler)

	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	oldSlog := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(oldSlog)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/bench", nil)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}
}
