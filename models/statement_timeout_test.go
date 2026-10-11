package models_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// statementRecorder is a GORM logger that records the SQL of every statement
// GORM runs, so a test can tell whether any SQL ran at all.
type statementRecorder struct {
	mu   sync.Mutex
	sqls []string
}

// LogMode returns the recorder unchanged; it records at every level.
func (r *statementRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

// Info ignores informational messages.
func (r *statementRecorder) Info(context.Context, string, ...any) {}

// Warn ignores warnings.
func (r *statementRecorder) Warn(context.Context, string, ...any) {}

// Error ignores error messages.
func (r *statementRecorder) Error(context.Context, string, ...any) {}

// Trace records the SQL of one statement.
func (r *statementRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sqls = append(r.sqls, sql)
}

// statements returns a copy of the SQL recorded so far.
func (r *statementRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.sqls...)
}

func TestWithinStatementTimeout_RefusesANonPositiveTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "zero", timeout: 0},
		{name: "negative", timeout: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := &statementRecorder{}
			db := newTxDB(t).Session(&gorm.Session{Logger: recorder})
			ran := false

			err := models.WithinStatementTimeout(t.Context(), db, tt.timeout, func(context.Context, *gorm.DB) error {
				ran = true

				return nil
			})
			require.ErrorIs(t, err, models.ErrValidation)
			assert.False(t, ran, "fn must not run")
			assert.Empty(t, recorder.statements(), "no SQL runs before the timeout is checked")

			// The recorder does see a statement, so the empty check above means something.
			require.NoError(t, db.Exec("SELECT 1").Error)
			assert.Len(t, recorder.statements(), 1)
		})
	}
}

func TestWithinStatementTimeout_RunsInATransactionWithNoBoundOnSQLite(t *testing.T) {
	t.Parallel()

	t.Run("fn gets the transaction in its context", func(t *testing.T) {
		t.Parallel()

		db := newTxDB(t)

		var (
			fromCtx *gorm.DB
			handed  *gorm.DB
		)

		err := models.WithinStatementTimeout(t.Context(), db, time.Second, func(fnCtx context.Context, tx *gorm.DB) error {
			fromCtx = models.DBFrom(fnCtx, nil)
			handed = tx

			return nil
		})
		require.NoError(t, err)
		require.NotNil(t, fromCtx, "fn's context carries a handle")
		assert.Same(t, handed, fromCtx, "fn's context carries the handle fn is given")

		_, isTx := fromCtx.Statement.ConnPool.(gorm.TxCommitter)
		assert.True(t, isTx, "the handle is a transaction")
	})

	t.Run("an error from fn rolls back and comes back unchanged", func(t *testing.T) {
		t.Parallel()

		db := newTxDB(t)

		err := models.WithinStatementTimeout(t.Context(), db, time.Second, func(fnCtx context.Context, _ *gorm.DB) error {
			if createErr := models.DBFrom(fnCtx, nil).Create(&txRow{ID: "rolled-back", Label: "a"}).Error; createErr != nil {
				return createErr
			}

			return errBoom
		})
		require.ErrorIs(t, err, errBoom)
		assert.Same(t, errBoom, err, "fn's error comes back unchanged")

		var count int64
		require.NoError(t, db.Model(&txRow{}).Where("id = ?", "rolled-back").Count(&count).Error)
		assert.Zero(t, count, "the insert is rolled back")
	})

	t.Run("a nil from fn commits with no bound", func(t *testing.T) {
		t.Parallel()

		recorder := &statementRecorder{}
		db := newTxDB(t).Session(&gorm.Session{Logger: recorder})

		err := models.WithinStatementTimeout(t.Context(), db, time.Second, func(fnCtx context.Context, _ *gorm.DB) error {
			return models.DBFrom(fnCtx, nil).Create(&txRow{ID: "committed", Label: "b"}).Error
		})
		require.NoError(t, err)

		statements := recorder.statements()
		require.Len(t, statements, 1, "only fn's insert runs; nothing sets a bound")
		assert.Contains(t, statements[0], "INSERT")

		var got txRow
		require.NoError(t, db.First(&got, "id = ?", "committed").Error)
		assert.Equal(t, "b", got.Label)
	})
}
