//go:build integration

// Package testutil's PostgreSQL harness boots (or connects to) a real
// PostgreSQL instance for integration tests. Unlike the SQLite helpers it is
// gated behind the "integration" build tag so unit-test runs stay fast and
// dependency-free.
//
// This module ships no schema of its own: callers AutoMigrate the models they
// exercise. NewPostgresTestDB returns a shared, truncated-between-tests handle;
// NewPostgresIsolatedDB returns a private schema safe for t.Parallel.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDatabaseURLEnv is the env var that, when set, points the harness at an
// already-running PostgreSQL instance instead of booting a container.
const TestDatabaseURLEnv = "FOUNDATION_TEST_DATABASE_URL"

// postgresImage is the PostgreSQL image booted when no DSN is supplied.
const postgresImage = "postgres:17-alpine"

// containerBootTimeout bounds the cold-boot of the Postgres container.
const containerBootTimeout = 60 * time.Second

// Shared per-test-binary Postgres state. The container (or DSN connection) is
// created once under pgOnce and reused by every NewPostgresTestDB call; the
// container is reaped at process exit by the testcontainers Ryuk reaper.
var (
	pgOnce sync.Once //nolint:gochecknoglobals // one resource per test binary
	pgDB   *gorm.DB  //nolint:gochecknoglobals // shared test database handle
	pgDSN  string    //nolint:gochecknoglobals // resolved DSN, reused by isolated schemas
	pgErr  error     //nolint:gochecknoglobals,errname // mutable setup-failure state, not a sentinel
	pgSkip string    //nolint:gochecknoglobals // non-empty => skip reason
)

// NewPostgresTestDB returns a *gorm.DB bound to a shared PostgreSQL database.
//
// The first call in a test binary either connects to FOUNDATION_TEST_DATABASE_URL
// or boots a testcontainers-go Postgres container (~10s cold). Every subsequent
// call truncates all public tables (RESTART IDENTITY CASCADE) so each test
// starts from an empty, identity-reset schema. Callers own their schema via
// AutoMigrate.
//
// When neither a DSN nor a container runtime is available the test is skipped
// with a clear message rather than failed.
func NewPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	pgOnce.Do(initPostgresTestDB)

	if pgSkip != "" {
		t.Skip(pgSkip)
	}

	if pgErr != nil {
		t.Fatalf("postgres test database unavailable: %v", pgErr)
	}

	truncateAllTables(t, pgDB)

	return pgDB
}

// initPostgresTestDB establishes the shared database once: it connects to a
// supplied DSN or boots a container and resets the public schema. It records a
// skip reason instead of an error when no runtime is available.
func initPostgresTestDB() {
	ctx := context.Background()

	dsn := os.Getenv(TestDatabaseURLEnv)
	if dsn == "" {
		bootedDSN, skip, err := bootPostgresContainer(ctx)
		switch {
		case skip != "":
			pgSkip = skip
			return
		case err != nil:
			pgErr = err
			return
		}

		dsn = bootedDSN
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		pgErr = fmt.Errorf("connect to the Postgres test database: %w", err)
		return
	}

	// Reset the public schema so each run starts clean whether the target is a
	// fresh container or a reused DSN that already carries a schema.
	if err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE`).Error; err != nil {
		pgErr = fmt.Errorf("reset public schema: %w", err)
		return
	}

	if err = db.Exec(`CREATE SCHEMA public`).Error; err != nil {
		pgErr = fmt.Errorf("recreate public schema: %w", err)
		return
	}

	pgDB = db
	pgDSN = dsn
}

// pgIsolatedSeq disambiguates schema names across parallel calls within the
// same test binary.
var pgIsolatedSeq atomic.Uint64 //nolint:gochecknoglobals // parallel-safe schema sequencer

// NewPostgresIsolatedDB returns a *gorm.DB bound to a freshly created, empty
// Postgres schema with its own connection pool, so callers can use t.Parallel
// safely even though the underlying database is shared with sibling tests.
// Callers AutoMigrate the models they exercise into the isolated schema.
func NewPostgresIsolatedDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Reuse NewPostgresTestDB's once-init (skip/fatal handling, resolved DSN).
	base := NewPostgresTestDB(t)

	name := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), pgIsolatedSeq.Add(1))
	if err := base.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`).Error; err != nil {
		t.Fatalf("drop schema %s: %v", name, err)
	}

	if err := base.Exec(`CREATE SCHEMA ` + name).Error; err != nil {
		t.Fatalf("create schema %s: %v", name, err)
	}

	db, err := gorm.Open(postgres.Open(withSearchPath(pgDSN, name)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open schema-scoped connection for %s: %v", name, err)
	}

	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}

		_ = base.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`).Error
	})

	return db
}

// withSearchPath appends a search_path runtime parameter to a Postgres URL DSN
// so every connection on the resulting pool resolves unqualified names to the
// given schema first.
func withSearchPath(dsn, schemaName string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}

	return dsn + sep + "search_path=" + schemaName
}

// bootPostgresContainer starts a PostgreSQL container and returns its DSN. When
// no container runtime is reachable it returns a non-empty skip reason so the
// suite degrades gracefully.
func bootPostgresContainer(ctx context.Context) (dsn, skip string, err error) {
	bootCtx, cancel := context.WithTimeout(ctx, containerBootTimeout)
	defer cancel()

	ctr, runErr := tcpostgres.Run(
		bootCtx, postgresImage,
		tcpostgres.WithDatabase("foundation_test"),
		tcpostgres.WithUsername("foundation"),
		tcpostgres.WithPassword("foundation"),
		tcpostgres.BasicWaitStrategies(),
	)
	if runErr != nil {
		if isContainerRuntimeUnavailable(runErr) {
			return "", fmt.Sprintf(
				"no container runtime and %s is not set; skipping the Postgres suite "+
					"(start Docker or set %s to run it): %v",
				TestDatabaseURLEnv, TestDatabaseURLEnv, runErr,
			), nil
		}

		return "", "", fmt.Errorf("boot Postgres container: %w", runErr)
	}

	// The container is reaped at process exit by the testcontainers Ryuk reaper;
	// no long-lived reference is required to keep it alive.
	connStr, connErr := ctr.ConnectionString(ctx, "sslmode=disable")
	if connErr != nil {
		return "", "", fmt.Errorf("resolve container connection string: %w", connErr)
	}

	return connStr, "", nil
}

// isContainerRuntimeUnavailable reports whether err indicates that no Docker /
// container runtime is reachable, as opposed to a genuine boot failure.
func isContainerRuntimeUnavailable(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, sig := range []string{
		"cannot connect to the docker daemon",
		"docker daemon is not running",
		"failed to find any matching",
		"rootless docker not found",
		"docker: command not found",
		"no such file or directory",
		"connection refused",
	} {
		if strings.Contains(msg, sig) {
			return true
		}
	}

	return false
}

// truncateAllTables empties every public table, resetting identity sequences,
// so each test starts from a clean slate. A schema with no tables is a no-op.
func truncateAllTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	var tables []string
	if err := db.Raw(
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`,
	).Scan(&tables).Error; err != nil {
		t.Fatalf("list public tables: %v", err)
	}

	if len(tables) == 0 {
		return
	}

	quoted := make([]string, len(tables))
	for i, name := range tables {
		quoted[i] = `"` + name + `"`
	}

	stmt := `TRUNCATE ` + strings.Join(quoted, ", ") + ` RESTART IDENTITY CASCADE`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}
