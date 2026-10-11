package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
)

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
