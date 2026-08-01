package testutil_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/testutil"
)

// TestRecordingHandlerCapturesMessageLevelAndAttrs proves each record is a map
// carrying the message, the level string, and one key per structured attribute —
// the shape tests assert against.
func TestRecordingHandlerCapturesMessageLevelAndAttrs(t *testing.T) {
	t.Parallel()
	rec := &testutil.RecordingHandler{}
	logger := slog.New(rec)

	logger.Warn("drain timed out", "in_flight", 3, "runner", 0)
	logger.Info("done")

	records := rec.Records()
	require.Len(t, records, 2)

	assert.Equal(t, "drain timed out", records[0]["msg"])
	assert.Equal(t, slog.LevelWarn.String(), records[0]["level"])
	assert.EqualValues(t, 3, records[0]["in_flight"])
	assert.EqualValues(t, 0, records[0]["runner"])

	assert.Equal(t, "done", records[1]["msg"])
	assert.Equal(t, slog.LevelInfo.String(), records[1]["level"])
}

// TestRecordingHandlerEnabledAndDelegators covers the always-enabled predicate
// and the WithAttrs/WithGroup passthroughs that return the handler unchanged.
func TestRecordingHandlerEnabledAndDelegators(t *testing.T) {
	t.Parallel()
	rec := &testutil.RecordingHandler{}

	assert.True(t, rec.Enabled(context.Background(), slog.LevelDebug))
	assert.Same(t, rec, rec.WithAttrs([]slog.Attr{slog.String("k", "v")}))
	assert.Same(t, rec, rec.WithGroup("group"))
}

// TestRecordingHandlerRecordsReturnsSliceCopy proves Records hands back a slice
// snapshot, so a caller appending to the returned slice cannot grow or corrupt
// the handler's own log.
func TestRecordingHandlerRecordsReturnsSliceCopy(t *testing.T) {
	t.Parallel()
	rec := &testutil.RecordingHandler{}
	logger := slog.New(rec)
	logger.Info("first")

	snapshot := rec.Records()
	snapshot = append(snapshot, map[string]any{"msg": "injected"})
	assert.Len(t, snapshot, 2)

	logger.Info("second")
	current := rec.Records()
	require.Len(t, current, 2, "the handler's log grew only from real records, not the caller's append")
	assert.Equal(t, "first", current[0]["msg"])
	assert.Equal(t, "second", current[1]["msg"])
}
