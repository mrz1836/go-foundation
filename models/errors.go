package models

import "errors"

// Sentinel errors for model operations.
// Use errors.Is() to check for these errors in handlers and services.
var (
	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrValidation indicates the input data failed validation.
	ErrValidation = errors.New("validation failed")

	// ErrDuplicateKey indicates a unique constraint violation occurred.
	ErrDuplicateKey = errors.New("duplicate key violation")

	// ErrForeignKey indicates a foreign key constraint violation occurred.
	ErrForeignKey = errors.New("foreign key constraint violated")

	// ErrDatabaseError indicates a general database operation failure.
	ErrDatabaseError = errors.New("database operation failed")

	// ErrCascadeDelete indicates deletion was blocked due to dependent records.
	ErrCascadeDelete = errors.New("cannot delete: dependent records exist")

	// ErrInvalidID indicates the provided ID is not a valid UUID.
	ErrInvalidID = errors.New("invalid id format")

	// ErrHook indicates a hook execution failed.
	ErrHook = errors.New("hook error")
)

// ValidationError wraps ErrValidation with a specific message.
type ValidationError struct {
	Field   string
	Message string
}

// Error returns the formatted validation error message.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}

	return e.Message
}

// Unwrap returns the underlying sentinel error for errors.Is() support.
func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

// NewValidationError creates a new validation error with field and message.
func NewValidationError(field, message string) error {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
