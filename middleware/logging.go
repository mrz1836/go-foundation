// Package middleware provides production-ready HTTP middleware for serverless
// Lambda functions and local HTTP servers. All log output is structured JSON so
// entries are queryable with log aggregators (e.g. CloudWatch Logs Insights)
// without parsing.
package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maxErrorBodySize = 4096 // Keep at most 4KB of error response

type contextKey string

const requestIDContextKey contextKey = "request_id"

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDContextKey).(string); ok {
		return id
	}

	return ""
}

// requestID extracts the request ID from the request headers.
// It checks X-Request-ID first, then falls back to X-Amzn-Request-Id.
func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}

	return r.Header.Get("X-Amzn-Request-Id")
}

// responseWriter wraps http.ResponseWriter to capture the status code and
// response body for error responses only (4xx/5xx), avoiding unbounded memory
// allocation for successful responses.
type responseWriter struct {
	http.ResponseWriter

	statusCode    int
	body          strings.Builder
	isError       bool
	headerWritten bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.headerWritten {
		return
	}

	rw.headerWritten = true
	rw.statusCode = code
	rw.isError = code >= http.StatusBadRequest
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap returns the underlying ResponseWriter, allowing http.ResponseController
// to access http.Flusher, http.Hijacker, and other optional interfaces.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.isError {
		return rw.ResponseWriter.Write(b)
	}

	remaining := maxErrorBodySize - rw.body.Len()
	if remaining > 0 {
		if len(b) > remaining {
			_, _ = rw.body.Write(b[:remaining])
		} else {
			_, _ = rw.body.Write(b)
		}
	}

	return rw.ResponseWriter.Write(b)
}

// LoggingMiddleware logs structured JSON entries for every request and response.
// Request entries include method, path, source IP, user agent, and request ID.
// Response entries add HTTP status, latency (ms), and an extracted error message
// for 4xx/5xx responses. Log level follows HTTP status: info/warn/error.
//
// Compatible with any net/http handler, including chi routers.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := requestID(r)

		// Store request ID in context for downstream handlers.
		ctx := context.WithValue(r.Context(), requestIDContextKey, reqID)
		r = r.WithContext(ctx)

		slog.Info("Request started",
			slog.String("type", "request"),
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("source_ip", r.RemoteAddr),
			slog.String("user_agent", r.Header.Get("User-Agent")),
		)

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default if WriteHeader is never called
		}

		next.ServeHTTP(wrapped, r)

		latency := time.Since(start).Milliseconds()

		level := slog.LevelInfo
		if wrapped.statusCode >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if wrapped.statusCode >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		args := []any{
			slog.String("type", "response"),
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Int64("latency", latency),
		}
		if wrapped.statusCode >= http.StatusBadRequest {
			errStr := extractErrorMessage(wrapped.body.String())
			if errStr != "" {
				args = append(args, slog.String("error_message", errStr))
			}
		}

		slog.Log(ctx, level, "Request completed", args...)
	})
}

// extractErrorMessage extracts a human-readable error string from a JSON
// response body. It checks common error keys ("error", "message",
// "error_message") in order and returns the first string value found.
// Returns an empty string if the body is empty, not JSON, or has no error key.
func extractErrorMessage(body string) string {
	if body == "" {
		return ""
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return ""
	}

	for _, key := range []string{"error", "message", "error_message"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}

	return ""
}
