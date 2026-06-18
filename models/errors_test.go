package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

func TestValidationError_Error_WithField(t *testing.T) {
	t.Parallel()

	err := &models.ValidationError{
		Field:   "email",
		Message: "must be valid email",
	}

	assert.Equal(t, "email: must be valid email", err.Error())
}

func TestValidationError_Error_WithoutField(t *testing.T) {
	t.Parallel()

	err := &models.ValidationError{
		Field:   "",
		Message: "validation failed",
	}

	assert.Equal(t, "validation failed", err.Error())
}

func TestValidationError_Unwrap(t *testing.T) {
	t.Parallel()

	err := &models.ValidationError{
		Field:   "name",
		Message: "required",
	}

	assert.ErrorIs(t, err, models.ErrValidation)
}

func TestNewValidationError(t *testing.T) {
	t.Parallel()

	err := models.NewValidationError("password", "too short")

	var valErr *models.ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Equal(t, "password", valErr.Field)
	assert.Equal(t, "too short", valErr.Message)
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	// Verify all sentinel errors are distinct
	sentinels := []error{
		models.ErrNotFound,
		models.ErrValidation,
		models.ErrDuplicateKey,
		models.ErrForeignKey,
		models.ErrDatabaseError,
		models.ErrCascadeDelete,
		models.ErrInvalidID,
		models.ErrHook,
	}

	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i != j {
				assert.NotErrorIs(t, err1, err2, "errors should be distinct: %v vs %v", err1, err2)
			}
		}
	}
}
