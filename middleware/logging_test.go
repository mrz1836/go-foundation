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
	"testing"

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
	if jsonStart == -1 {
		t.Fatalf("no JSON object found in log line: %q", string(line))
	}

	var entry logEntry
	if err := json.Unmarshal(line[jsonStart:], &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nlog line: %s", err, string(line))
	}

	return entry
}

func assertRequestEntry(t *testing.T, entry logEntry, wantRequestID, wantUserAgent, wantLevel string) {
	t.Helper()

	if entry.Type != "request" {
		t.Errorf("type = %q, want 'request'", entry.Type)
	}

	if entry.RequestID != wantRequestID {
		t.Errorf("request_id = %q, want %q", entry.RequestID, wantRequestID)
	}

	if entry.UserAgent != wantUserAgent {
		t.Errorf("user_agent = %q, want %q", entry.UserAgent, wantUserAgent)
	}

	if entry.Level != wantLevel {
		t.Errorf("level = %q, want %q", entry.Level, wantLevel)
	}
}

func assertResponseEntry(t *testing.T, entry logEntry, wantStatus int, wantLevel string) {
	t.Helper()

	if entry.Type != "response" {
		t.Errorf("type = %q, want 'response'", entry.Type)
	}

	if entry.Status != wantStatus {
		t.Errorf("status = %d, want %d", entry.Status, wantStatus)
	}

	if entry.Level != wantLevel {
		t.Errorf("level = %q, want %q", entry.Level, wantLevel)
	}

	if entry.Latency < 0 {
		t.Errorf("latency = %d, want >= 0", entry.Latency)
	}
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

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	lines := bytes.Split([]byte(output), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log lines, got %d\noutput: %s", len(lines), output)
	}

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
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log lines, got %d", len(lines))
	}

	respEntry := parseLogEntry(t, lines[1])
	if respEntry.Level != "WARN" {
		t.Errorf("level = %q, want 'WARN'", respEntry.Level)
	}

	if respEntry.ErrorMessage != "resource not found" {
		t.Errorf("error_message = %q, want 'resource not found'", respEntry.ErrorMessage)
	}
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

	if respEntry.Level != "ERROR" {
		t.Errorf("level = %q, want 'ERROR'", respEntry.Level)
	}

	if respEntry.ErrorMessage != "database connection timeout" {
		t.Errorf("error_message = %q, want 'database connection timeout'", respEntry.ErrorMessage)
	}
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
	if len(lines) < 1 {
		t.Fatal("expected at least 1 log line")
	}

	entry := parseLogEntry(t, lines[0])
	if entry.Path != maliciousPath {
		t.Errorf("path = %q, want %q", entry.Path, maliciousPath)
	}

	// Confirm no injected top-level key appeared
	var raw map[string]any

	jsonStart := bytes.Index(lines[0], []byte("{"))
	if err := json.Unmarshal(lines[0][jsonStart:], &raw); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}

	if _, ok := raw["injected"]; ok {
		t.Error("log injection succeeded: 'injected' key found at top level")
	}
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
	if entry.RequestID != "amzn-req-xyz" {
		t.Errorf("request_id = %q, want 'amzn-req-xyz'", entry.RequestID)
	}
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
	if respEntry.ErrorMessage != "" {
		t.Errorf("error_message = %q, want empty for empty body", respEntry.ErrorMessage)
	}
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
	if respEntry.ErrorMessage != "" {
		t.Errorf("error_message = %q, want empty for non-JSON body", respEntry.ErrorMessage)
	}
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
	if respEntry.ErrorMessage != "validation failed" {
		t.Errorf("error_message = %q, want 'validation failed'", respEntry.ErrorMessage)
	}
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
	if respEntry.ErrorMessage != "" {
		t.Errorf("error_message = %q, want empty for non-string error value", respEntry.ErrorMessage)
	}
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
	if respEntry.ErrorMessage != "" {
		t.Errorf("error_message = %q, want empty when no recognized key present", respEntry.ErrorMessage)
	}
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

	if capturedID != "ctx-req-id" {
		t.Errorf("RequestIDFromContext = %q, want 'ctx-req-id'", capturedID)
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	t.Parallel()

	id := middleware.RequestIDFromContext(context.Background())
	if id != "" {
		t.Errorf("RequestIDFromContext on empty context = %q, want empty", id)
	}
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
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}

	// The logged status should also be 400
	lines := bytes.Split([]byte(output), []byte("\n"))

	respEntry := parseLogEntry(t, lines[1])
	if respEntry.Status != http.StatusBadRequest {
		t.Errorf("logged status = %d, want 400", respEntry.Status)
	}
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

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}

	var panicEntry struct {
		RequestID string `json:"request_id"`
	}

	lines := bytes.Split([]byte(output), []byte("\n"))

	jsonStart := bytes.Index(lines[0], []byte("{"))
	if jsonStart == -1 {
		t.Fatalf("no JSON in panic log: %q", string(lines[0]))
	}

	if err := json.Unmarshal(lines[0][jsonStart:], &panicEntry); err != nil {
		t.Fatalf("unmarshal panic log: %v", err)
	}

	if panicEntry.RequestID != "amzn-panic-id" {
		t.Errorf("request_id = %q, want 'amzn-panic-id'", panicEntry.RequestID)
	}
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

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
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

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

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
	if jsonStart == -1 {
		t.Fatalf("no JSON in panic log: %q", string(lines[0]))
	}

	if err := json.Unmarshal(lines[0][jsonStart:], &panicEntry); err != nil {
		t.Fatalf("unmarshal panic log: %v", err)
	}

	if panicEntry.Level != "ERROR" {
		t.Errorf("level = %q, want 'ERROR'", panicEntry.Level)
	}

	if panicEntry.Type != "panic" {
		t.Errorf("type = %q, want 'panic'", panicEntry.Type)
	}

	if panicEntry.RequestID != "panic-req-id" {
		t.Errorf("request_id = %q, want 'panic-req-id'", panicEntry.RequestID)
	}

	if panicEntry.Stack == "" {
		t.Error("stack should be non-empty")
	}
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
