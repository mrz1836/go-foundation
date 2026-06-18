package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/models"
)

func TestDefaultDBLogger_LogOperation_AllOperationTypes(t *testing.T) {
	t.Parallel()

	logger := &models.DefaultDBLogger{}
	ctx := context.Background()

	operationTypes := []string{
		models.OpCreate,
		models.OpUpdate,
		models.OpDelete,
		models.OpSoftDelete,
		models.OpRestore,
		models.OpQuery,
	}

	for _, opType := range operationTypes {
		t.Run(opType, func(t *testing.T) {
			t.Parallel()

			op := models.DBOperation{
				Timestamp:    time.Now().UTC(),
				Type:         opType,
				Model:        "test_model",
				ID:           "test-id-123",
				DurationMS:   10,
				RowsAffected: 1,
				RequestID:    "req-123",
			}

			// Should not panic
			assert.NotPanics(t, func() {
				logger.LogOperation(ctx, op)
			})
		})
	}
}

func TestDefaultDBLogger_LogOperation_WithError(t *testing.T) {
	t.Parallel()

	logger := &models.DefaultDBLogger{}
	ctx := context.Background()

	op := models.DBOperation{
		Timestamp:    time.Now().UTC(),
		Type:         models.OpCreate,
		Model:        "test_model",
		ID:           "test-id-123",
		DurationMS:   5,
		RowsAffected: 0,
		Error:        "database connection failed",
		RequestID:    "req-456",
	}

	// Should not panic when logging errors
	assert.NotPanics(t, func() {
		logger.LogOperation(ctx, op)
	})
}

func TestDefaultDBLogger_LogOperation_WithSpecialCharactersInError(t *testing.T) {
	t.Parallel()

	logger := &models.DefaultDBLogger{}
	ctx := context.Background()

	testCases := []struct {
		name  string
		error string
	}{
		{"double quotes", `error with "quotes"`},
		{"backslashes", `error with \ backslash`},
		{"newlines", "error with\nnewline"},
		{"carriage return", "error with\rcarriage return"},
		{"tabs", "error with\ttab"},
		{"mixed special chars", "error \"with\"\n\\special\t\rchars"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := models.DBOperation{
				Timestamp: time.Now().UTC(),
				Type:      models.OpQuery,
				Model:     "test",
				Error:     tc.error,
			}

			// Should not panic with special characters in error
			assert.NotPanics(t, func() {
				logger.LogOperation(ctx, op)
			})
		})
	}
}

func TestDefaultDBLogger_LogOperation_EmptyFields(t *testing.T) {
	t.Parallel()

	logger := &models.DefaultDBLogger{}
	ctx := context.Background()

	// Test with minimal/empty fields
	op := models.DBOperation{
		Timestamp: time.Now().UTC(),
		Type:      models.OpQuery,
		Model:     "",
		ID:        "",
	}

	assert.NotPanics(t, func() {
		logger.LogOperation(ctx, op)
	})
}

func TestNopDBLogger_LogOperation(t *testing.T) {
	t.Parallel()

	logger := &models.NopDBLogger{}
	ctx := context.Background()

	op := models.DBOperation{
		Timestamp:    time.Now().UTC(),
		Type:         models.OpCreate,
		Model:        "test_model",
		ID:           "test-id",
		DurationMS:   100,
		RowsAffected: 1,
		Error:        "some error",
		RequestID:    "req-123",
	}

	// NopDBLogger should do nothing but not panic
	assert.NotPanics(t, func() {
		logger.LogOperation(ctx, op)
	})
}

func TestGetRequestIDFromContext_WithValidRequestID(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), models.RequestIDKey, "test-request-id-123")

	result := models.GetRequestIDFromContext(ctx)

	assert.Equal(t, "test-request-id-123", result)
}

func TestGetRequestIDFromContext_WithoutRequestID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	result := models.GetRequestIDFromContext(ctx)

	assert.Empty(t, result)
}

func TestGetRequestIDFromContext_WithNonStringValue(t *testing.T) {
	t.Parallel()

	// Test with a non-string value in context
	ctx := context.WithValue(context.Background(), models.RequestIDKey, 12345)

	result := models.GetRequestIDFromContext(ctx)

	assert.Empty(t, result, "should return empty string for non-string values")
}

func TestGetRequestIDFromContext_WithNilValue(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), models.RequestIDKey, nil)

	result := models.GetRequestIDFromContext(ctx)

	assert.Empty(t, result)
}

func TestDBOperation_Constants(t *testing.T) {
	t.Parallel()

	// Verify operation constants are defined correctly
	assert.Equal(t, "create", models.OpCreate)
	assert.Equal(t, "update", models.OpUpdate)
	assert.Equal(t, "delete", models.OpDelete)
	assert.Equal(t, "soft_delete", models.OpSoftDelete)
	assert.Equal(t, "restore", models.OpRestore)
	assert.Equal(t, "query", models.OpQuery)
}

func TestDBOperation_Struct(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	op := models.DBOperation{
		Timestamp:    now,
		Type:         models.OpCreate,
		Model:        "city",
		ID:           "uuid-123",
		DurationMS:   42,
		RowsAffected: 1,
		Error:        "",
		RequestID:    "req-abc",
	}

	assert.Equal(t, now, op.Timestamp)
	assert.Equal(t, models.OpCreate, op.Type)
	assert.Equal(t, "city", op.Model)
	assert.Equal(t, "uuid-123", op.ID)
	assert.Equal(t, int64(42), op.DurationMS)
	assert.Equal(t, int64(1), op.RowsAffected)
	assert.Empty(t, op.Error)
	assert.Equal(t, "req-abc", op.RequestID)
}
