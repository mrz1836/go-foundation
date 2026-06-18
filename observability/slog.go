// Package observability centralizes log/metric/trace setup so every binary
// (local server, Lambda, daemon, snapshot tool, build tooling) configures slog
// the same way.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

// DefaultLevelEnvVar is the environment variable consulted by [Init] to choose
// the log level. Consuming services that prefer a project-specific variable
// (for example "MYAPP_LOG_LEVEL") can call [InitWithLevelVar] instead.
const DefaultLevelEnvVar = "LOG_LEVEL"

// Init constructs a JSON slog.Logger, installs it as the process default, and
// returns it for callers that want a direct handle. The level is read from the
// LOG_LEVEL environment variable (debug|info|warn|error); unrecognized or unset
// values fall back to info.
//
// Call Init once at the start of main(). All log/slog package-level functions
// then route through the configured handler.
func Init(env string) *slog.Logger {
	return InitWithLevelVar(env, DefaultLevelEnvVar)
}

// InitWithLevelVar behaves like [Init] but reads the level from a caller-chosen
// environment variable, allowing a service to honor its own prefixed variable
// (for example "MYAPP_LOG_LEVEL") without leaking project naming into this
// module.
func InitWithLevelVar(env, levelEnvVar string) *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: levelFromEnv(levelEnvVar),
	})).With("env", env)
	slog.SetDefault(logger)

	return logger
}

// levelFromEnv parses the named environment variable into a slog.Level,
// defaulting to info.
func levelFromEnv(levelEnvVar string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(levelEnvVar))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
