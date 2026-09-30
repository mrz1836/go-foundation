package db

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/config"
)

// probeRecord is a tiny model used to prove that writes and reads still behave
// correctly with each optional GORM behavior enabled.
type probeRecord struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

// TableName pins the probe table name so the round-trip test is self-contained.
func (probeRecord) TableName() string { return "db_option_probe" }

// newMemSQLite opens an in-memory SQLite connection through NewConnection with
// the given options. In-memory SQLite gives each connection its own database, so
// this is for flag/ping assertions only — use newFileSQLite when data must
// survive across pooled connections.
func newMemSQLite(t *testing.T, opts ...Option) *gorm.DB {
	t.Helper()

	cfg := &config.WriteDatabaseConfig{Driver: driverSQLite, Database: ":memory:"}

	database, err := NewConnection(cfg, opts...)
	require.NoError(t, err, "NewConnection failed")

	t.Cleanup(func() {
		if sqlDB, derr := database.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})

	return database
}

// newFileSQLite opens a file-backed SQLite connection under t.TempDir() so writes
// are visible across the pool's connections.
func newFileSQLite(t *testing.T, opts ...Option) *gorm.DB {
	t.Helper()

	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: filepath.Join(t.TempDir(), "probe.db"),
	}

	database, err := NewConnection(cfg, opts...)
	require.NoError(t, err, "NewConnection failed")

	t.Cleanup(func() {
		if sqlDB, derr := database.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})

	return database
}

// TestNewConnection_GORMConfigMatrix verifies every combination of the two
// options resolves to the expected gorm.Config flags, and that passing none
// preserves GORM's defaults (both off).
func TestNewConnection_GORMConfigMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []Option
		wantSkip    bool
		wantPrepare bool
	}{
		{"defaults", nil, false, false},
		{"skip only", []Option{WithSkipDefaultTransaction(true)}, true, false},
		{"prepare only", []Option{WithPrepareStmt(true)}, false, true},
		{"both", []Option{WithSkipDefaultTransaction(true), WithPrepareStmt(true)}, true, true},
		{"explicit false", []Option{WithSkipDefaultTransaction(false), WithPrepareStmt(false)}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			database := newMemSQLite(t, tt.opts...)

			assert.Equal(t, tt.wantSkip, database.SkipDefaultTransaction)
			assert.Equal(t, tt.wantPrepare, database.PrepareStmt)
		})
	}
}

// TestNewConnection_OptionOrderLastWins documents that options are applied in
// order, so a later value overrides an earlier one.
func TestNewConnection_OptionOrderLastWins(t *testing.T) {
	t.Parallel()

	database := newMemSQLite(t, WithSkipDefaultTransaction(true), WithSkipDefaultTransaction(false))

	assert.False(t, database.SkipDefaultTransaction,
		"a later option must override an earlier one: want SkipDefaultTransaction=false")
}

// TestNewConnection_NilOptionIgnored verifies a nil Option in the variadic slice
// is skipped (not a panic) while real options still apply.
func TestNewConnection_NilOptionIgnored(t *testing.T) {
	t.Parallel()

	database := newMemSQLite(t, nil, WithPrepareStmt(true), nil)

	assert.True(t, database.PrepareStmt, "expected PrepareStmt to be enabled alongside nil options")
}

// TestNewConnection_WritesRoundTripWithOptions is the behavioral guard: with each
// option (and both) enabled, a migrate + two inserts + read-back must succeed and
// persist. The second insert also exercises the prepared-statement cache.
func TestNewConnection_WritesRoundTripWithOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts []Option
	}{
		{"skip default transaction", []Option{WithSkipDefaultTransaction(true)}},
		{"prepare stmt", []Option{WithPrepareStmt(true)}},
		{"both", []Option{WithSkipDefaultTransaction(true), WithPrepareStmt(true)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertWritesRoundTrip(t, tc.opts...)
		})
	}
}

// assertWritesRoundTrip migrates the probe model, inserts two rows, and verifies
// both persist — proving writes still behave correctly with the given options.
// The second insert exercises the prepared-statement cache when it is enabled.
func assertWritesRoundTrip(t *testing.T, opts ...Option) {
	t.Helper()

	database := newFileSQLite(t, opts...)

	require.NoError(t, database.AutoMigrate(&probeRecord{}), "AutoMigrate failed")

	for _, name := range []string{"alpha", "beta"} {
		require.NoErrorf(t, database.Create(&probeRecord{Name: name}).Error, "Create(%q) failed", name)
	}

	var got []probeRecord
	require.NoError(t, database.Find(&got).Error, "Find failed")

	require.Len(t, got, 2, "expected 2 persisted rows")
}

// TestNewConnection_OptionsPreservePoolConfig verifies options do not disturb the
// connection-pool configuration applied from the DatabaseConfig.
func TestNewConnection_OptionsPreservePoolConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     ":memory:",
		MaxOpenConns: 7,
	}

	database, err := NewConnection(cfg, WithSkipDefaultTransaction(true), WithPrepareStmt(true))
	require.NoError(t, err, "NewConnection failed")

	sqlDB, err := database.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
	}()

	stats := sqlDB.Stats()
	assert.Equal(t, 7, stats.MaxOpenConnections, "expected MaxOpenConnections=7 to be preserved")
}

// TestNewConnection_LoggerDefault verifies that without a logger option, or with a
// nil one, the connection keeps GORM's default logger.
func TestNewConnection_LoggerDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []Option
	}{
		{"no option", nil},
		{"nil logger", []Option{WithLogger(nil)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			database := newMemSQLite(t, tt.opts...)

			assert.Same(t, gormlogger.Default, database.Logger, "expected GORM's default logger")
		})
	}
}

// TestNewConnection_LoggerInstalled verifies a supplied logger is the one the
// connection logs through, and that a later WithLogger overrides an earlier one.
func TestNewConnection_LoggerInstalled(t *testing.T) {
	t.Parallel()

	first := gormlogger.NewSlogLogger(slog.New(slog.DiscardHandler), gormlogger.Config{})
	second := gormlogger.NewSlogLogger(slog.New(slog.DiscardHandler), gormlogger.Config{})

	t.Run("supplied logger", func(t *testing.T) {
		t.Parallel()

		database := newMemSQLite(t, WithLogger(first))

		assert.Same(t, first, database.Logger)
	})

	t.Run("last option wins", func(t *testing.T) {
		t.Parallel()

		database := newMemSQLite(t, WithLogger(first), WithLogger(second))

		assert.Same(t, second, database.Logger)
	})
}

// TestNewConnection_LoggerWithholdsBoundValues is the behavioral guard for the
// recommended use: a slog JSON logger with ParameterizedQueries logs a statement as
// a single JSON record whose SQL keeps its placeholder and carries no bound value.
func TestNewConnection_LoggerWithholdsBoundValues(t *testing.T) {
	t.Parallel()

	const sentinel = "bound-value-sentinel@example.com"

	var buf bytes.Buffer

	database := newMemSQLite(t, WithLogger(gormlogger.NewSlogLogger(
		slog.New(slog.NewJSONHandler(&buf, nil)),
		gormlogger.Config{LogLevel: gormlogger.Info, ParameterizedQueries: true},
	)))
	buf.Reset() // assert on the query below only, not on anything logged at open

	var n int64
	require.NoError(t, database.Table("sqlite_master").Where("name = ?", sentinel).Count(&n).Error)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1, "expected exactly one log record: %q", buf.String())

	var record struct {
		Msg   string `json:"msg"`
		Trace struct {
			SQL string `json:"sql"`
		} `json:"trace"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record), "the record must be a JSON line")
	assert.Equal(t, "SQL executed", record.Msg)
	assert.Contains(t, record.Trace.SQL, "name = ?", "the logged SQL must keep its placeholder")
	assert.NotContains(t, buf.String(), sentinel, "the bound value must never reach the log")
}
