package observability_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/observability"
)

func TestInit_ReturnsLoggerAndSetsDefault(t *testing.T) {
	logger := observability.Init("test")
	require.NotNil(t, logger)
	assert.Equal(t, logger, slog.Default())
}

func TestInitWithLevelVar_HonorsCustomVarAndLevels(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		enabled slog.Level
		want    bool
	}{
		{name: "debug enables debug", value: "debug", enabled: slog.LevelDebug, want: true},
		{name: "warn disables info", value: "warn", enabled: slog.LevelInfo, want: false},
		{name: "warn enables warn", value: "warn", enabled: slog.LevelWarn, want: true},
		{name: "error disables warn", value: "error", enabled: slog.LevelWarn, want: false},
		{name: "unset defaults to info", value: "", enabled: slog.LevelInfo, want: true},
		{name: "garbage defaults to info", value: "loud", enabled: slog.LevelInfo, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const customVar = "FOUNDATION_TEST_LOG_LEVEL"
			if tc.value != "" {
				t.Setenv(customVar, tc.value)
			}

			logger := observability.InitWithLevelVar("test", customVar)
			require.NotNil(t, logger)
			assert.Equal(t, tc.want, logger.Enabled(t.Context(), tc.enabled))
		})
	}
}
