// Package constants provides shared, domain-agnostic HTTP constants — header
// names, content types, CORS values, and standardized error codes and messages —
// used by the foundation HTTP, Lambda, and response helpers.
//
// Only generic values live here. Project-specific naming (application names,
// health-check messages, infrastructure constants) belongs in the consuming
// service, never in this module.
package constants

// HTTP Headers
const (
	// HeaderContentType is the Content-Type HTTP header
	HeaderContentType = "Content-Type"

	// HeaderAccessControlAllowOrigin is the CORS Access-Control-Allow-Origin header
	HeaderAccessControlAllowOrigin = "Access-Control-Allow-Origin"

	// HeaderXRequestID is the primary request-id header set by clients/gateways
	HeaderXRequestID = "X-Request-ID"

	// HeaderXAmznRequestID is the API Gateway / AWS request-id header
	HeaderXAmznRequestID = "X-Amzn-Request-Id"
)

// Structured-log and metadata field names
const (
	// FieldRequestID is the log attribute and JSON metadata key for the request id
	FieldRequestID = "request_id"
)

// Content Type Values
const (
	// ContentTypeJSON is the application/json content type
	ContentTypeJSON = "application/json"
)

// CORS Values
const (
	// CORSAllowOriginAll allows all origins (*)
	CORSAllowOriginAll = "*"
)

// Error Codes - standardized error codes used across the API
const (
	// ErrorCodeNotFound indicates the requested resource was not found
	ErrorCodeNotFound = "NOT_FOUND"

	// ErrorCodeBadRequest indicates the request was malformed or invalid
	ErrorCodeBadRequest = "BAD_REQUEST"

	// ErrorCodeMethodNotAllowed indicates the HTTP method is not supported
	ErrorCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"

	// ErrorCodeInternalError indicates an internal server error occurred
	ErrorCodeInternalError = "INTERNAL_ERROR"

	// ErrorCodeUnauthorized indicates authentication is required
	ErrorCodeUnauthorized = "UNAUTHORIZED"

	// ErrorCodeForbidden indicates the caller lacks permission
	ErrorCodeForbidden = "FORBIDDEN"

	// ErrorCodeConflict indicates a resource conflict (e.g., optimistic concurrency)
	ErrorCodeConflict = "CONFLICT"

	// ErrorCodeTooManyRequests indicates the caller has been rate-limited
	ErrorCodeTooManyRequests = "TOO_MANY_REQUESTS"
)

// Error Messages - standardized error messages used across the API
const (
	// ErrorMessageNotFound is the message for 404 errors
	ErrorMessageNotFound = "The requested endpoint does not exist"

	// ErrorMessageBadRequest is the default message for 400 errors
	ErrorMessageBadRequest = "The request was invalid"

	// ErrorMessageMethodNotAllowed is the message for 405 errors
	ErrorMessageMethodNotAllowed = "The HTTP method is not supported for this endpoint"

	// ErrorMessageInternalError is the message for 500 errors
	ErrorMessageInternalError = "An internal error occurred"

	// ErrorMessageUnauthorized is the message for 401 errors
	ErrorMessageUnauthorized = "Authentication is required"

	// ErrorMessageForbidden is the message for 403 errors
	ErrorMessageForbidden = "You do not have permission to access this resource"

	// ErrorMessageEncodingFailed is the message when JSON encoding fails
	ErrorMessageEncodingFailed = "Failed to encode response"

	// ErrorMessageConflict is the default message for 409 errors
	ErrorMessageConflict = "The resource conflicts with the current state"

	// ErrorMessageTooManyRequests is the message for 429 errors
	ErrorMessageTooManyRequests = "Too many requests, please try again later"
)

// StaticErrorJSON assembles the minimal {"error":..,"code":..} body used as a
// last-resort fallback when the normal JSON marshaler is unavailable (a marshal
// failure). message and code must be free of characters that require JSON
// escaping; the error constants in this package satisfy that.
func StaticErrorJSON(message, code string) string {
	return `{"error":"` + message + `","code":"` + code + `"}`
}
