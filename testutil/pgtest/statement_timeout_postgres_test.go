//go:build integration

package pgtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// sqlStateQueryCanceled is the SQLSTATE PostgreSQL reports for a statement it
// canceled, whether for statement_timeout or for a cancel the client asked for.
const sqlStateQueryCanceled = "57014"

// cancelRequestDeadline is how long a cancel-request connection waits for the
// server to answer a cancel before it gives up on the connection.
const cancelRequestDeadline = 10 * time.Second

// showStatementTimeout reads statement_timeout as SHOW reports it on db's
// connection.
func showStatementTimeout(db *gorm.DB) (string, error) {
	var value string
	err := db.Raw("SHOW statement_timeout").Scan(&value).Error

	return value, err
}

// newCancelRequestDB returns a second handle on db's schema whose driver, when
// a context ends mid-statement, sends the server a cancel request instead of
// closing the connection (pgx's default). The server then cancels the statement
// and reports SQLSTATE 57014, as it does for statement_timeout.
func newCancelRequestDB(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()

	dialector, ok := db.Dialector.(*postgres.Dialector)
	require.True(t, ok, "the handle uses the PostgreSQL dialector")

	config, err := pgx.ParseConfig(dialector.DSN)
	require.NoError(t, err)

	config.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, DeadlineDelay: cancelRequestDeadline}
	}

	sqlDB := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = sqlDB.Close() })

	cancelDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	return cancelDB
}

func TestStatementTimeout_PostgresCancelsALongStatement(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)

	err := models.WithinStatementTimeout(t.Context(), db, 100*time.Millisecond, func(_ context.Context, tx *gorm.DB) error {
		return tx.Exec("SELECT pg_sleep(1)").Error
	})
	require.ErrorIs(t, err, models.ErrStatementTimeout)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "the driver's error stays in the chain")
	assert.Equal(t, sqlStateQueryCanceled, pgErr.Code)
}

func TestStatementTimeout_PostgresBoundEndsWithTheTransaction(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)

	// One connection, so the reads after the call see the connection the
	// transaction ran on.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	var inside string

	err = models.WithinStatementTimeout(t.Context(), db, 100*time.Millisecond, func(_ context.Context, tx *gorm.DB) error {
		var showErr error
		inside, showErr = showStatementTimeout(tx)

		return showErr
	})
	require.NoError(t, err)
	assert.Equal(t, "100ms", inside, "the bound applies inside fn")

	after, err := showStatementTimeout(db)
	require.NoError(t, err)
	assert.Equal(t, "0", after, "the bound ends with the transaction")
	require.NoError(t, db.Exec("SELECT pg_sleep(0.2)").Error, "a statement longer than the bound runs after the call")
}

// nestedTimeoutRun is what a call nested in an outer transaction saw: fn's
// statement_timeout, the outer transaction's afterwards, and the call's error.
type nestedTimeoutRun struct {
	inside string
	after  string
	err    error
}

// runNestedStatementTimeout opens a transaction bounded at 5s, calls
// WithinStatementTimeout in it with 100ms and an fn that runs statement, and
// reads statement_timeout inside fn and in the outer transaction afterwards.
func runNestedStatementTimeout(t *testing.T, statement string) nestedTimeoutRun {
	t.Helper()

	db := pgtest.NewPostgresIsolatedDB(t)

	var run nestedTimeoutRun

	err := db.WithContext(t.Context()).Transaction(func(outer *gorm.DB) error {
		if setErr := outer.Exec("SET LOCAL statement_timeout = '5s'").Error; setErr != nil {
			return setErr
		}

		// db is the pool; the transaction ctx carries wins.
		run.err = models.WithinStatementTimeout(models.WithTx(t.Context(), outer), db, 100*time.Millisecond,
			func(_ context.Context, tx *gorm.DB) error {
				var showErr error
				if run.inside, showErr = showStatementTimeout(tx); showErr != nil {
					return showErr
				}

				return tx.Exec(statement).Error
			})

		var showErr error
		run.after, showErr = showStatementTimeout(outer)

		return showErr
	})
	require.NoError(t, err)

	return run
}

func TestStatementTimeout_PostgresRestoresTheOuterValueWhenNested(t *testing.T) {
	t.Run("fn succeeds", func(t *testing.T) {
		run := runNestedStatementTimeout(t, "SELECT 1")
		require.NoError(t, run.err)
		assert.Equal(t, "100ms", run.inside, "the inner bound applies inside fn")
		assert.Equal(t, "5s", run.after, "the outer transaction has its own bound back")
	})

	t.Run("fn times out", func(t *testing.T) {
		run := runNestedStatementTimeout(t, "SELECT pg_sleep(1)")
		require.ErrorIs(t, run.err, models.ErrStatementTimeout)
		assert.Equal(t, "100ms", run.inside, "the inner bound applies inside fn")
		assert.Equal(t, "5s", run.after, "the rollback to the savepoint gives the outer transaction its bound back")
	})
}

func TestStatementTimeout_PostgresCancelledContextReturnsItsOwnError(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)

	tests := []struct {
		name string
		db   *gorm.DB
		// serverCancels is whether the driver asks the server to cancel the
		// statement, so the statement fails with SQLSTATE 57014 although no
		// bound expired.
		serverCancels bool
	}{
		{name: "the driver closes the connection", db: db},
		{name: "the driver asks the server to cancel", db: newCancelRequestDB(t, db), serverCancels: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			var stmtErr error

			err := models.WithinStatementTimeout(ctx, tt.db, 5*time.Second, func(_ context.Context, tx *gorm.DB) error {
				timer := time.AfterFunc(100*time.Millisecond, cancel)
				defer timer.Stop()

				stmtErr = tx.Exec("SELECT pg_sleep(1)").Error

				return stmtErr
			})
			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, models.ErrStatementTimeout)
			require.Error(t, stmtErr, "the statement was canceled")

			if tt.serverCancels {
				var pgErr *pgconn.PgError
				require.ErrorAs(t, stmtErr, &pgErr, "the server reported the cancel")
				assert.Equal(t, sqlStateQueryCanceled, pgErr.Code)
			}
		})
	}
}

func TestStatementTimeout_PostgresRoundsUpToWholeMilliseconds(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)

	var inside string

	err := models.WithinStatementTimeout(t.Context(), db, 1500*time.Microsecond, func(_ context.Context, tx *gorm.DB) error {
		var showErr error
		inside, showErr = showStatementTimeout(tx)

		return showErr
	})
	require.NoError(t, err)
	assert.Equal(t, "2ms", inside)
}
