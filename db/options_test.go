package db

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

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
	if err != nil {
		t.Fatalf("NewConnection failed: %v", err)
	}

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
	if err != nil {
		t.Fatalf("NewConnection failed: %v", err)
	}

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

			if database.SkipDefaultTransaction != tt.wantSkip {
				t.Errorf("SkipDefaultTransaction = %v, want %v", database.SkipDefaultTransaction, tt.wantSkip)
			}

			if database.PrepareStmt != tt.wantPrepare {
				t.Errorf("PrepareStmt = %v, want %v", database.PrepareStmt, tt.wantPrepare)
			}
		})
	}
}

// TestNewConnection_OptionOrderLastWins documents that options are applied in
// order, so a later value overrides an earlier one.
func TestNewConnection_OptionOrderLastWins(t *testing.T) {
	t.Parallel()

	database := newMemSQLite(t, WithSkipDefaultTransaction(true), WithSkipDefaultTransaction(false))

	if database.SkipDefaultTransaction {
		t.Error("a later option must override an earlier one: want SkipDefaultTransaction=false")
	}
}

// TestNewConnection_NilOptionIgnored verifies a nil Option in the variadic slice
// is skipped (not a panic) while real options still apply.
func TestNewConnection_NilOptionIgnored(t *testing.T) {
	t.Parallel()

	database := newMemSQLite(t, nil, WithPrepareStmt(true), nil)

	if !database.PrepareStmt {
		t.Error("expected PrepareStmt to be enabled alongside nil options")
	}
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

	if err := database.AutoMigrate(&probeRecord{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	for _, name := range []string{"alpha", "beta"} {
		if err := database.Create(&probeRecord{Name: name}).Error; err != nil {
			t.Fatalf("Create(%q) failed: %v", name, err)
		}
	}

	var got []probeRecord
	if err := database.Find(&got).Error; err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 persisted rows, got %d", len(got))
	}
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
	if err != nil {
		t.Fatalf("NewConnection failed: %v", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}
	}()

	if stats := sqlDB.Stats(); stats.MaxOpenConnections != 7 {
		t.Errorf("expected MaxOpenConnections=7 to be preserved, got %d", stats.MaxOpenConnections)
	}
}
