package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Sentinel errors declared at package scope keep err113 happy (no dynamic
// errors.New inside test functions) and let assertions use ErrorIs.
var (
	errPGBoom  = errors.New("boom")
	errPGBoot  = errors.New("boot boom")
	errPGOpen  = errors.New("open boom")
	errPGReset = errors.New("reset boom")
)

// fakeExecutor is an in-memory pgExecutor that records executed statements and
// returns scripted results, so every branch of the harness logic is reachable
// without a real database.
type fakeExecutor struct {
	tables   []string
	listErr  error
	execErr  map[string]error // keyed by statement => error to return
	executed []string
}

func (f *fakeExecutor) listPublicTables() ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	return f.tables, nil
}

func (f *fakeExecutor) exec(sql string) error {
	f.executed = append(f.executed, sql)
	if f.execErr != nil {
		if err, ok := f.execErr[sql]; ok {
			return err
		}
	}

	return nil
}

func TestWithSearchPath(t *testing.T) {
	tests := []struct {
		name   string
		dsn    string
		schema string
		want   string
	}{
		{
			name:   "no existing query appends with ?",
			dsn:    "postgres://host:5432/db",
			schema: "t_1",
			want:   "postgres://host:5432/db?search_path=t_1",
		},
		{
			name:   "existing query appends with &",
			dsn:    "postgres://host:5432/db?sslmode=disable",
			schema: "t_2",
			want:   "postgres://host:5432/db?sslmode=disable&search_path=t_2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, withSearchPath(tt.dsn, tt.schema))
		})
	}
}

func TestIsContainerRuntimeUnavailable(t *testing.T) {
	unavailable := []string{
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock",
		"the docker daemon is not running",
		"failed to find any matching runtime",
		"rootless Docker not found",
		"docker: command not found",
		"stat /var/run/docker.sock: no such file or directory",
		"dial tcp: connection refused",
		"permission denied while trying to connect to the Docker API",
	}
	for _, msg := range unavailable {
		t.Run("unavailable: "+msg, func(t *testing.T) {
			assert.True(t, isContainerRuntimeUnavailable(errors.New(msg))) //nolint:err113 // table-driven message fixtures
		})
	}

	t.Run("genuine failure is not treated as unavailable", func(t *testing.T) {
		assert.False(t, isContainerRuntimeUnavailable(errPGBoom))
	})
}

func TestTruncateStatement(t *testing.T) {
	t.Run("no tables yields empty statement", func(t *testing.T) {
		assert.Empty(t, truncateStatement(nil))
		assert.Empty(t, truncateStatement([]string{}))
	})

	t.Run("quotes and joins table names", func(t *testing.T) {
		got := truncateStatement([]string{"widgets", "gadgets"})
		assert.Equal(t, `TRUNCATE "widgets", "gadgets" RESTART IDENTITY CASCADE`, got)
	})
}

func TestIsolatedSchemaName(t *testing.T) {
	assert.Equal(t, "t_123_7", isolatedSchemaName(123, 7))
}

func TestResetPublicSchema(t *testing.T) {
	t.Run("runs drop then create", func(t *testing.T) {
		e := &fakeExecutor{}
		require.NoError(t, resetPublicSchema(e))
		assert.Equal(t, []string{dropPublicSchemaSQL, createPublicSchemaSQL}, e.executed)
	})

	t.Run("drop error surfaces", func(t *testing.T) {
		e := &fakeExecutor{execErr: map[string]error{dropPublicSchemaSQL: errPGBoom}}
		err := resetPublicSchema(e)
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "reset public schema")
	})

	t.Run("create error surfaces", func(t *testing.T) {
		e := &fakeExecutor{execErr: map[string]error{createPublicSchemaSQL: errPGBoom}}
		err := resetPublicSchema(e)
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "recreate public schema")
	})
}

func TestCreateIsolatedSchema(t *testing.T) {
	dropSQL := `DROP SCHEMA IF EXISTS t_1 CASCADE`
	createSQL := `CREATE SCHEMA t_1`

	t.Run("runs drop then create", func(t *testing.T) {
		e := &fakeExecutor{}
		require.NoError(t, createIsolatedSchema(e, "t_1"))
		assert.Equal(t, []string{dropSQL, createSQL}, e.executed)
	})

	t.Run("drop error surfaces", func(t *testing.T) {
		e := &fakeExecutor{execErr: map[string]error{dropSQL: errPGBoom}}
		err := createIsolatedSchema(e, "t_1")
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "drop schema t_1")
	})

	t.Run("create error surfaces", func(t *testing.T) {
		e := &fakeExecutor{execErr: map[string]error{createSQL: errPGBoom}}
		err := createIsolatedSchema(e, "t_1")
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "create schema t_1")
	})
}

func TestTruncatePublicTables(t *testing.T) {
	t.Run("list error surfaces", func(t *testing.T) {
		e := &fakeExecutor{listErr: errPGBoom}
		err := truncatePublicTables(e)
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "list public tables")
	})

	t.Run("no tables is a no-op", func(t *testing.T) {
		e := &fakeExecutor{tables: nil}
		require.NoError(t, truncatePublicTables(e))
		assert.Empty(t, e.executed)
	})

	t.Run("truncates listed tables", func(t *testing.T) {
		e := &fakeExecutor{tables: []string{"widgets"}}
		require.NoError(t, truncatePublicTables(e))
		assert.Equal(t, []string{`TRUNCATE "widgets" RESTART IDENTITY CASCADE`}, e.executed)
	})

	t.Run("truncate error surfaces", func(t *testing.T) {
		stmt := `TRUNCATE "widgets" RESTART IDENTITY CASCADE`
		e := &fakeExecutor{tables: []string{"widgets"}, execErr: map[string]error{stmt: errPGBoom}}
		err := truncatePublicTables(e)
		require.ErrorIs(t, err, errPGBoom)
		assert.Contains(t, err.Error(), "truncate tables")
	})
}

func TestSetupPostgres(t *testing.T) {
	noEnv := func(string) string { return "" }
	suppliedEnv := func(string) string { return "postgres://supplied/db" }
	okExec := &fakeExecutor{}
	stubDB := &gorm.DB{}
	okOpen := func(string) (*gorm.DB, pgExecutor, error) { return stubDB, okExec, nil } //nolint:unparam // matches postgresDeps.open signature

	t.Run("uses supplied DSN and resets schema", func(t *testing.T) {
		var openedDSN string
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: suppliedEnv,
			boot: func(context.Context) (string, string, error) {
				t.Error("boot must not be called when a DSN is supplied")
				return "", "", nil
			},
			open: func(dsn string) (*gorm.DB, pgExecutor, error) {
				openedDSN = dsn
				return stubDB, &fakeExecutor{}, nil
			},
		})
		require.NoError(t, got.err)
		assert.Empty(t, got.skip)
		assert.Equal(t, "postgres://supplied/db", got.dsn)
		assert.Equal(t, "postgres://supplied/db", openedDSN)
		assert.Same(t, stubDB, got.db)
	})

	t.Run("boots a container when no DSN is set", func(t *testing.T) {
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: noEnv,
			boot:   func(context.Context) (string, string, error) { return "postgres://booted/db", "", nil },
			open:   okOpen,
		})
		require.NoError(t, got.err)
		assert.Equal(t, "postgres://booted/db", got.dsn)
	})

	t.Run("records skip when no runtime is available", func(t *testing.T) {
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: noEnv,
			boot:   func(context.Context) (string, string, error) { return "", "no docker", nil },
			open:   okOpen,
		})
		require.NoError(t, got.err)
		assert.Equal(t, "no docker", got.skip)
		assert.Nil(t, got.db)
	})

	t.Run("records error on genuine boot failure", func(t *testing.T) {
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: noEnv,
			boot:   func(context.Context) (string, string, error) { return "", "", errPGBoot },
			open:   okOpen,
		})
		require.ErrorIs(t, got.err, errPGBoot)
	})

	t.Run("wraps open failures", func(t *testing.T) {
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: suppliedEnv,
			open:   func(string) (*gorm.DB, pgExecutor, error) { return nil, nil, errPGOpen },
		})
		require.ErrorIs(t, got.err, errPGOpen)
		assert.Contains(t, got.err.Error(), "connect to the Postgres test database")
	})

	t.Run("surfaces schema reset failures", func(t *testing.T) {
		failingExec := &fakeExecutor{execErr: map[string]error{dropPublicSchemaSQL: errPGReset}}
		got := setupPostgres(context.Background(), postgresDeps{
			getenv: suppliedEnv,
			open:   func(string) (*gorm.DB, pgExecutor, error) { return stubDB, failingExec, nil },
		})
		require.ErrorIs(t, got.err, errPGReset)
	})
}

func TestGormExecutor(t *testing.T) {
	open := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		return db
	}

	t.Run("exec runs a statement", func(t *testing.T) {
		e := gormExecutor{db: open(t)}
		require.NoError(t, e.exec("CREATE TABLE widgets (id INTEGER)"))
	})

	t.Run("listPublicTables returns rows from a pg_tables shim", func(t *testing.T) {
		db := open(t)
		// SQLite has no pg_tables; stand one up so the success path is exercised.
		require.NoError(t, db.Exec(`CREATE TABLE pg_tables (schemaname TEXT, tablename TEXT)`).Error)
		require.NoError(t, db.Exec(`INSERT INTO pg_tables VALUES ('public', 'widgets')`).Error)

		tables, err := gormExecutor{db: db}.listPublicTables()
		require.NoError(t, err)
		assert.Equal(t, []string{"widgets"}, tables)
	})

	t.Run("listPublicTables surfaces query errors", func(t *testing.T) {
		// No pg_tables relation exists, so the query fails.
		_, err := gormExecutor{db: open(t)}.listPublicTables()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pg_tables")
	})
}
