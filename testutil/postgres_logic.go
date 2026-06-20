// Package testutil's PostgreSQL harness logic lives here, deliberately WITHOUT
// the "integration" build tag, so the branchy setup/reset/skip logic is unit
// testable with a fake executor and injected seams — no container or live
// database required. The heavy, testcontainers-backed wiring that drives this
// logic stays in postgres.go behind the "integration" tag.
package testutil

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// TestDatabaseURLEnv is the env var that, when set, points the harness at an
// already-running PostgreSQL instance instead of booting a container.
const TestDatabaseURLEnv = "FOUNDATION_TEST_DATABASE_URL"

// SQL executed to reset the shared public schema between runs.
const (
	dropPublicSchemaSQL   = `DROP SCHEMA IF EXISTS public CASCADE`
	createPublicSchemaSQL = `CREATE SCHEMA public`
)

// pgExecutor is the minimal database surface the Postgres harness needs to
// reset, list, and truncate schemas. Abstracting it behind an interface lets
// the harness logic be exercised with a fake, so error and edge branches are
// reachable without a real database.
type pgExecutor interface {
	listPublicTables() ([]string, error)
	exec(sql string) error
}

// gormExecutor adapts a *gorm.DB to pgExecutor.
type gormExecutor struct{ db *gorm.DB }

// listPublicTables returns the names of every table in the public schema.
func (g gormExecutor) listPublicTables() ([]string, error) {
	var tables []string
	if err := g.db.Raw(
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`,
	).Scan(&tables).Error; err != nil {
		return nil, err
	}

	return tables, nil
}

// exec runs a statement and returns its error, if any.
func (g gormExecutor) exec(sql string) error {
	return g.db.Exec(sql).Error
}

// postgresDeps are the injectable seams of the shared-database setup. Supplying
// fakes makes every branch of setupPostgres reachable in a unit test.
type postgresDeps struct {
	// getenv resolves environment variables (os.Getenv in production).
	getenv func(string) string
	// boot returns a DSN, or a non-empty skip reason when no container runtime
	// is available, or an error on a genuine boot failure.
	boot func(ctx context.Context) (dsn, skip string, err error)
	// open connects to the resolved DSN and returns a handle plus its executor.
	open func(dsn string) (*gorm.DB, pgExecutor, error)
}

// postgresSetup is the resolved shared-database state. Exactly one of db, skip,
// or err is meaningful: db on success, skip when no runtime is available, err
// on a genuine failure.
type postgresSetup struct {
	db   *gorm.DB
	dsn  string
	skip string
	err  error
}

// setupPostgres resolves the shared Postgres test database: it connects to a
// supplied DSN or boots a container, then resets the public schema so each run
// starts clean. It records a skip reason (rather than an error) when no
// container runtime is available, letting the suite degrade gracefully.
func setupPostgres(ctx context.Context, d postgresDeps) postgresSetup {
	dsn := d.getenv(TestDatabaseURLEnv)
	if dsn == "" {
		bootedDSN, skip, err := d.boot(ctx)
		switch {
		case skip != "":
			return postgresSetup{skip: skip}
		case err != nil:
			return postgresSetup{err: err}
		}

		dsn = bootedDSN
	}

	db, exec, err := d.open(dsn)
	if err != nil {
		return postgresSetup{err: fmt.Errorf("connect to the Postgres test database: %w", err)}
	}

	if err = resetPublicSchema(exec); err != nil {
		return postgresSetup{err: err}
	}

	return postgresSetup{db: db, dsn: dsn}
}

// resetPublicSchema drops and recreates the public schema so each run starts
// from a clean slate whether the target is a fresh container or a reused DSN
// that already carries a schema.
func resetPublicSchema(e pgExecutor) error {
	if err := e.exec(dropPublicSchemaSQL); err != nil {
		return fmt.Errorf("reset public schema: %w", err)
	}

	if err := e.exec(createPublicSchemaSQL); err != nil {
		return fmt.Errorf("recreate public schema: %w", err)
	}

	return nil
}

// createIsolatedSchema drops any stale schema of the same name and creates a
// fresh one, giving t.Parallel callers a private namespace.
func createIsolatedSchema(e pgExecutor, name string) error {
	if err := e.exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`); err != nil {
		return fmt.Errorf("drop schema %s: %w", name, err)
	}

	if err := e.exec(`CREATE SCHEMA ` + name); err != nil {
		return fmt.Errorf("create schema %s: %w", name, err)
	}

	return nil
}

// truncatePublicTables empties every table in the public schema, resetting
// identity sequences, so each test starts from a clean slate. A schema with no
// tables is a no-op.
func truncatePublicTables(e pgExecutor) error {
	tables, err := e.listPublicTables()
	if err != nil {
		return fmt.Errorf("list public tables: %w", err)
	}

	stmt := truncateStatement(tables)
	if stmt == "" {
		return nil
	}

	if err = e.exec(stmt); err != nil {
		return fmt.Errorf("truncate tables: %w", err)
	}

	return nil
}

// truncateStatement builds a TRUNCATE ... RESTART IDENTITY CASCADE statement for
// the given tables, quoting each identifier. It returns "" when there are no
// tables so callers can skip the no-op exec.
func truncateStatement(tables []string) string {
	if len(tables) == 0 {
		return ""
	}

	quoted := make([]string, len(tables))
	for i, name := range tables {
		quoted[i] = `"` + name + `"`
	}

	return `TRUNCATE ` + strings.Join(quoted, ", ") + ` RESTART IDENTITY CASCADE`
}

// isolatedSchemaName builds a unique schema name from a timestamp (nanos) and a
// per-binary sequence so parallel callers never collide.
func isolatedSchemaName(nanos int64, seq uint64) string {
	return fmt.Sprintf("t_%d_%d", nanos, seq)
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

// isContainerRuntimeUnavailable reports whether err indicates that no Docker /
// container runtime is reachable (so the suite should skip), as opposed to a
// genuine boot failure (which should surface).
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
		"permission denied while trying to connect",
	} {
		if strings.Contains(msg, sig) {
			return true
		}
	}

	return false
}
