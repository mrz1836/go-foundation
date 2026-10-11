package models

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// pgCodeQueryCanceled is the SQLSTATE PostgreSQL reports when it cancels a
// statement: for statement_timeout, and for a cancel the client asked for.
const pgCodeQueryCanceled = "57014"

// postgresDialect is the dialect name GORM's PostgreSQL driver reports.
const postgresDialect = "postgres"

// WithinStatementTimeout runs fn in a transaction in which PostgreSQL cancels
// any statement that runs longer than timeout. Use it to bound work whose cost
// depends on its input, such as a search or a report, so that one slow
// statement cannot hold a connection for long.
//
// The handle is DBFrom(ctx, db): pass the read replica for reads
// (Repository.ReadDB) and the primary for writes. A transaction that ctx
// already carries wins, so a call made inside Transactor.WithinTx runs on that
// transaction's connection.
//
// It works in these steps:
//
//  1. A timeout that is zero or negative is refused, before any SQL runs and
//     without running fn, with an error wrapping ErrValidation.
//  2. It begins a transaction on the handle, or a savepoint when the handle is
//     already a transaction.
//  3. On PostgreSQL, in a savepoint, it reads the enclosing transaction's
//     statement_timeout. It then sets statement_timeout to timeout, in whole
//     milliseconds rounded up, for the rest of the transaction, with
//     set_config('statement_timeout', ..., true): the SET LOCAL form that takes
//     a bound parameter.
//  4. It runs fn with a context that carries the transaction (see WithTx) and
//     with the transaction handle. fn's queries must use one of them.
//  5. When fn returns nil in a savepoint, it sets the enclosing value back,
//     because GORM never releases a savepoint, so the bound would otherwise
//     outlive fn. When fn returns an error, the rollback to the savepoint
//     restores it. (With gorm.Config.DisableNestedTransaction there is no
//     savepoint, so after fn fails the enclosing transaction keeps the bound.)
//  6. It commits when fn returns nil, and rolls back otherwise.
//
// It returns nil when the transaction commits. On failure, a ctx that is done
// wins: it returns ctx.Err(). When PostgreSQL canceled a statement (SQLSTATE
// 57014), it returns an error wrapping both ErrStatementTimeout and the
// driver's error. Any other error, fn's own included, comes back unchanged.
// SQLSTATE 57014 also reports a cancel the client asked for, which a driver
// may send when ctx ends mid-statement, so ErrStatementTimeout is reported
// only while ctx is not done.
//
// On any other database there is no bound: fn still runs in a transaction, but
// nothing cancels a slow statement.
//
// Behind a connection proxy such as Amazon RDS Proxy, set_config pins the
// client connection to its database connection, though the bound still ends
// with the transaction. To bound statements and keep the proxy's multiplexing,
// set a role default (ALTER ROLE ... SET statement_timeout) or use the proxy's
// initialization query instead.
func WithinStatementTimeout(ctx context.Context, db *gorm.DB, timeout time.Duration,
	fn func(ctx context.Context, tx *gorm.DB) error,
) error {
	if timeout <= 0 {
		return fmt.Errorf("%w: the statement timeout must be positive", ErrValidation)
	}

	base := DBFrom(ctx, db)
	nested := inTransaction(base)
	limit := statementTimeoutMillis(timeout)

	err := base.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// tx.Name is the dialect name of tx's GORM dialector.
		if tx.Name() != postgresDialect {
			return fn(WithTx(ctx, tx), tx)
		}

		return runWithStatementTimeout(ctx, tx, limit, nested, fn)
	})

	return statementTimeoutResult(ctx, err)
}

// runWithStatementTimeout sets statement_timeout to limit for the rest of tx
// and runs fn. When nested, tx is a savepoint in an enclosing transaction: it
// reads the enclosing value first, and sets it back after fn succeeds.
func runWithStatementTimeout(ctx context.Context, tx *gorm.DB, limit string, nested bool,
	fn func(ctx context.Context, tx *gorm.DB) error,
) error {
	var previous string
	if nested {
		if err := tx.Raw("SELECT current_setting('statement_timeout')").Scan(&previous).Error; err != nil {
			return err
		}
	}

	if err := setStatementTimeout(tx, limit); err != nil {
		return err
	}

	if err := fn(WithTx(ctx, tx), tx); err != nil {
		return err
	}

	if nested {
		return setStatementTimeout(tx, previous)
	}

	return nil
}

// setStatementTimeout sets statement_timeout to value until tx ends. SET takes
// no bound parameter, so it uses set_config with is_local true, which is SET
// LOCAL.
func setStatementTimeout(tx *gorm.DB, value string) error {
	return tx.Exec("SELECT set_config('statement_timeout', ?, true)", value).Error
}

// statementTimeoutMillis renders timeout as whole milliseconds, rounded up and
// at least 1, so that a bound never ends early and a sub-millisecond one is not
// 0, which PostgreSQL reads as no bound. Dividing and then adding the remainder
// rounds up without overflowing near the largest time.Duration.
func statementTimeoutMillis(timeout time.Duration) string {
	ms := timeout / time.Millisecond
	if timeout%time.Millisecond != 0 {
		ms++
	}

	return strconv.FormatInt(int64(max(ms, 1)), 10)
}

// statementTimeoutResult maps the transaction's result: nil stays nil, a ctx
// that is done wins, a statement PostgreSQL canceled wraps
// ErrStatementTimeout, and any other error comes back unchanged.
func statementTimeoutResult(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	if isQueryCanceled(err) {
		return fmt.Errorf("%w: %w", ErrStatementTimeout, err)
	}

	return err
}

// isQueryCanceled reports whether err's chain holds a PostgreSQL error with
// SQLSTATE 57014.
func isQueryCanceled(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == pgCodeQueryCanceled
}
