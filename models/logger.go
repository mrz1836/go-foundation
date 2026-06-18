package models

import (
	"context"
	"log/slog"
	"time"
)

// DBLogger defines an interface for database operation logging.
// Implementations can use this for audit trails, metrics, or debugging.
type DBLogger interface {
	// LogOperation logs a database operation with context.
	LogOperation(ctx context.Context, op DBOperation)
}

// DBOperation represents a database operation for logging.
type DBOperation struct {
	// Timestamp is when the operation occurred (UTC)
	Timestamp time.Time `json:"timestamp"`

	// Type is the operation type: "create", "update", "delete", "query"
	Type string `json:"type"`

	// Model is the model name: "user", "account", "order"
	Model string `json:"model"`

	// ID is the record ID (if applicable)
	ID string `json:"id,omitempty"`

	// Duration is how long the operation took
	DurationMS int64 `json:"duration_ms,omitempty"`

	// RowsAffected is the number of rows affected (for write operations)
	RowsAffected int64 `json:"rows_affected,omitempty"`

	// Error is the error message (if operation failed)
	Error string `json:"error,omitempty"`

	// RequestID is the request ID from context (for correlation)
	RequestID string `json:"request_id,omitempty"`
}

// Operation type constants
const (
	OpCreate     = "create"
	OpUpdate     = "update"
	OpDelete     = "delete"
	OpSoftDelete = "soft_delete"
	OpRestore    = "restore"
	OpQuery      = "query"
)

// DefaultDBLogger forwards DB operations to slog.Default() as a single
// structured "db_operation" line. The handler installed by the binary
// (typically observability.Init) controls level filtering, output, and
// encoding.
type DefaultDBLogger struct{}

// LogOperation logs the database operation through slog.
func (l *DefaultDBLogger) LogOperation(ctx context.Context, op DBOperation) {
	attrs := []slog.Attr{
		slog.String("op", op.Type),
		slog.String("model", op.Model),
		slog.Int64("duration_ms", op.DurationMS),
		slog.Int64("rows", op.RowsAffected),
	}
	if op.ID != "" {
		attrs = append(attrs, slog.String("id", op.ID))
	}

	if op.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", op.RequestID))
	}

	level := slog.LevelInfo
	if op.Error != "" {
		level = slog.LevelError

		attrs = append(attrs, slog.String("error", op.Error))
	}

	slog.Default().LogAttrs(ctx, level, "db_operation", attrs...)
}

// NopDBLogger is a no-op logger for testing or when logging is disabled.
type NopDBLogger struct{}

// LogOperation does nothing.
func (l *NopDBLogger) LogOperation(_ context.Context, _ DBOperation) {}

// ContextKey type for context values.
type contextKey string

// RequestIDKey is the context key for request ID.
const RequestIDKey contextKey = "request_id"

// GetRequestIDFromContext extracts the request ID from context for log correlation.
func GetRequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}
