package constants_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/constants"
)

func TestHTTPConstants(t *testing.T) {
	t.Parallel()

	// Headers
	assert.Equal(t, "Content-Type", constants.HeaderContentType)
	assert.Equal(t, "Access-Control-Allow-Origin", constants.HeaderAccessControlAllowOrigin)

	// Content types
	assert.Equal(t, "application/json", constants.ContentTypeJSON)

	// CORS
	assert.Equal(t, "*", constants.CORSAllowOriginAll)

	// Error codes
	assert.Equal(t, "NOT_FOUND", constants.ErrorCodeNotFound)
	assert.Equal(t, "BAD_REQUEST", constants.ErrorCodeBadRequest)
	assert.Equal(t, "METHOD_NOT_ALLOWED", constants.ErrorCodeMethodNotAllowed)
	assert.Equal(t, "INTERNAL_ERROR", constants.ErrorCodeInternalError)
	assert.Equal(t, "UNAUTHORIZED", constants.ErrorCodeUnauthorized)
	assert.Equal(t, "FORBIDDEN", constants.ErrorCodeForbidden)
	assert.Equal(t, "CONFLICT", constants.ErrorCodeConflict)
	assert.Equal(t, "TOO_MANY_REQUESTS", constants.ErrorCodeTooManyRequests)

	// Error messages
	assert.Equal(t, "The requested endpoint does not exist", constants.ErrorMessageNotFound)
	assert.Equal(t, "The request was invalid", constants.ErrorMessageBadRequest)
	assert.Equal(t, "The HTTP method is not supported for this endpoint", constants.ErrorMessageMethodNotAllowed)
	assert.Equal(t, "An internal error occurred", constants.ErrorMessageInternalError)
	assert.Equal(t, "Authentication is required", constants.ErrorMessageUnauthorized)
	assert.Equal(t, "You do not have permission to access this resource", constants.ErrorMessageForbidden)
	assert.Equal(t, "Failed to encode response", constants.ErrorMessageEncodingFailed)
	assert.Equal(t, "The resource conflicts with the current state", constants.ErrorMessageConflict)
	assert.Equal(t, "Too many requests, please try again later", constants.ErrorMessageTooManyRequests)
}
