// Package httputil provides shared HTTP response helpers. All JSON responses,
// standard error shapes, and named error helpers live here so every Lambda
// function and HTTP server writes consistent, structured responses without
// duplicating boilerplate.
package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mrz1836/go-foundation/constants"
)

// ErrorResponse is the standard error shape returned by API endpoints.
// Consumers should check the Code field for machine-readable error classification.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON serializes data as JSON and writes it to w with the given HTTP status.
// Content-Type is always set to application/json. If marshaling fails, a 500
// with a static error body is written instead — the original status is discarded.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		slog.Error("httputil: failed to marshal JSON response", slog.Any("error", err))
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"` + constants.ErrorMessageEncodingFailed + `","code":"` + constants.ErrorCodeInternalError + `"}`))

		return
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// WriteError writes a JSON error response with the provided HTTP status, error
// code, human-readable message, and optional request ID.
func WriteError(w http.ResponseWriter, status int, code, message, requestID string) {
	WriteJSON(w, status, ErrorResponse{
		Error:     message,
		Code:      code,
		RequestID: requestID,
	})
}

// WriteNotFound writes a 404 Not Found response using the standard error code.
func WriteNotFound(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusNotFound, constants.ErrorCodeNotFound, constants.ErrorMessageNotFound, requestID)
}

// WriteBadRequest writes a 400 Bad Request response. The caller supplies the
// message so callers can describe which field or value was invalid.
func WriteBadRequest(w http.ResponseWriter, message, requestID string) {
	WriteError(w, http.StatusBadRequest, constants.ErrorCodeBadRequest, message, requestID)
}

// WriteInternalError writes a 500 Internal Server Error response.
func WriteInternalError(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusInternalServerError, constants.ErrorCodeInternalError, constants.ErrorMessageInternalError, requestID)
}

// WriteMethodNotAllowed writes a 405 Method Not Allowed response.
func WriteMethodNotAllowed(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusMethodNotAllowed, constants.ErrorCodeMethodNotAllowed, constants.ErrorMessageMethodNotAllowed, requestID)
}

// WriteUnauthorized writes a 401 Unauthorized response.
func WriteUnauthorized(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusUnauthorized, constants.ErrorCodeUnauthorized, constants.ErrorMessageUnauthorized, requestID)
}

// WriteForbidden writes a 403 Forbidden response.
func WriteForbidden(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusForbidden, constants.ErrorCodeForbidden, constants.ErrorMessageForbidden, requestID)
}

// WriteConflict writes a 409 Conflict response. The caller supplies the
// message so callers can describe the specific conflict.
func WriteConflict(w http.ResponseWriter, message, requestID string) {
	WriteError(w, http.StatusConflict, constants.ErrorCodeConflict, message, requestID)
}

// WriteTooManyRequests writes a 429 Too Many Requests response.
func WriteTooManyRequests(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusTooManyRequests, constants.ErrorCodeTooManyRequests, constants.ErrorMessageTooManyRequests, requestID)
}
