package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/mrz1836/go-foundation/httputil"
)

// RecoverMiddleware catches panics in downstream handlers, logs a structured
// entry with the full stack trace, and writes a 500 Internal Server Error
// response. Without this middleware a panic crashes the Lambda invocation
// and returns an opaque 502 from API Gateway.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				handlePanic(w, r, rec)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// handlePanic logs the recovered value and stack trace as structured JSON,
// then writes a standard 500 error response via httputil.
func handlePanic(w http.ResponseWriter, r *http.Request, rec any) {
	stack := debug.Stack()

	reqID := requestID(r)

	slog.Error("panic recovered",
		slog.String("type", "panic"),
		slog.String("request_id", reqID),
		slog.String("error", fmt.Sprintf("%v", rec)),
		slog.String("stack", string(stack)),
	)

	httputil.WriteInternalError(w, reqID)
}
