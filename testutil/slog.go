package testutil

import (
	"context"
	"log/slog"
	"sync"
)

// RecordingHandler captures the structured attributes of every log record, so a
// test can assert on the log's cadence and content rather than on its absence.
// Each record is a map with "msg" and "level" keys plus one key per attribute.
type RecordingHandler struct {
	mu   sync.Mutex
	logs []map[string]any
}

// Enabled reports the handler is always enabled.
func (*RecordingHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle records one log record's message, level, and attributes.
func (h *RecordingHandler) Handle(_ context.Context, rec slog.Record) error {
	entry := map[string]any{"msg": rec.Message, "level": rec.Level.String()}
	rec.Attrs(func(a slog.Attr) bool {
		entry[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, entry)
	return nil
}

// WithAttrs returns the handler unchanged.
func (h *RecordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup returns the handler unchanged.
func (h *RecordingHandler) WithGroup(string) slog.Handler { return h }

// Records returns a copy of what has been logged so far.
func (h *RecordingHandler) Records() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]any(nil), h.logs...)
}
