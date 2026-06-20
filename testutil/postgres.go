//go:build integration

// Package testutil's PostgreSQL harness boots (or connects to) a real
// PostgreSQL instance for integration tests. Unlike the SQLite helpers it is
// gated behind the "integration" build tag so unit-test runs stay fast and
// dependency-free.
//
// The branchy setup/reset/skip LOGIC lives in postgres_logic.go (untagged, unit
// tested with a fake executor). This file holds only the heavy, I/O-bound glue:
// booting a testcontainers Postgres, wiring the real seams into that logic, and
// the *testing.T entry points.
//
// This module ships no schema of its own: callers AutoMigrate the models they
// exercise. NewPostgresTestDB returns a shared, truncated-between-tests handle;
// NewPostgresIsolatedDB returns a private schema safe for t.Parallel.
package testutil

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// postgresImage is the PostgreSQL image booted when no DSN is supplied.
const postgresImage = "postgres:17-alpine"

// containerBootTimeout bounds the cold-boot of the Postgres container.
const containerBootTimeout = 60 * time.Second

// Shared per-test-binary Postgres state. The container (or DSN connection) is
// created once under pgOnce and reused by every NewPostgresTestDB call; the
// container is reaped at process exit by the testcontainers Ryuk reaper.
var (
	pgOnce  sync.Once     //nolint:gochecknoglobals // one resource per test binary
	pgSetup postgresSetup //nolint:gochecknoglobals // resolved shared-database state
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

	if pgSetup.skip != "" {
		t.Skip(pgSetup.skip)
	}

	if pgSetup.err != nil {
		t.Fatalf("postgres test database unavailable: %v", pgSetup.err)
	}

	truncateAllTables(t, pgSetup.db)

	return pgSetup.db
}

// initPostgresTestDB establishes the shared database once by driving
// setupPostgres with the real environment, container booter, and gorm opener.
func initPostgresTestDB() {
	pgSetup = setupPostgres(context.Background(), postgresDeps{
		getenv: os.Getenv,
		boot:   bootPostgresContainer,
		open:   openPostgres,
	})
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

	name := isolatedSchemaName(time.Now().UnixNano(), pgIsolatedSeq.Add(1))
	if err := createIsolatedSchema(gormExecutor{db: base}, name); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}

	db, err := gorm.Open(postgres.Open(withSearchPath(pgSetup.dsn, name)), &gorm.Config{
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

// truncateAllTables empties every public table, failing the test on error.
func truncateAllTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := truncatePublicTables(gormExecutor{db: db}); err != nil {
		t.Fatalf("%v", err)
	}
}

// openPostgres connects to the given DSN with a silent logger and returns the
// handle plus its executor. It is the production seam for postgresDeps.open.
func openPostgres(dsn string) (*gorm.DB, pgExecutor, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, err
	}

	return db, gormExecutor{db: db}, nil
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
