package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mrz1836/go-foundation/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type txRow struct {
	ID    string `gorm:"primaryKey"`
	Label string
}

var (
	errBoom      = errors.New("boom")
	errInnerBoom = errors.New("inner boom")
)

func newTxDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&txRow{}))

	return db
}

func TestTransactor_CommitsWhenFnReturnsNil(t *testing.T) {
	t.Parallel()

	db := newTxDB(t)
	tx := models.NewTransactor(db)

	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		return models.DBFrom(ctx, db).Create(&txRow{ID: "row-1", Label: "a"}).Error
	})
	require.NoError(t, err)

	var got txRow
	require.NoError(t, db.First(&got, "id = ?", "row-1").Error)
	assert.Equal(t, "a", got.Label)
}

func TestTransactor_RollsBackWhenFnReturnsError(t *testing.T) {
	t.Parallel()

	db := newTxDB(t)
	tx := models.NewTransactor(db)

	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if createErr := models.DBFrom(ctx, db).Create(&txRow{ID: "row-2", Label: "b"}).Error; createErr != nil {
			return createErr
		}

		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	var count int64
	require.NoError(t, db.Model(&txRow{}).Where("id = ?", "row-2").Count(&count).Error)
	assert.Equal(t, int64(0), count, "rolled-back row must not be visible")
}

func TestTransactor_NestedCallsUseSavepoints(t *testing.T) {
	t.Parallel()

	db := newTxDB(t)
	tx := models.NewTransactor(db)

	err := tx.WithinTx(context.Background(), func(outerCtx context.Context) error {
		if err := models.DBFrom(outerCtx, db).Create(&txRow{ID: "outer", Label: "outer"}).Error; err != nil {
			return err
		}

		// Inner WithinTx must roll back independently via savepoint without
		// killing the outer transaction.
		innerErr := tx.WithinTx(outerCtx, func(innerCtx context.Context) error {
			if err := models.DBFrom(innerCtx, db).Create(&txRow{ID: "inner", Label: "inner"}).Error; err != nil {
				return err
			}

			return errInnerBoom
		})
		if !errors.Is(innerErr, errInnerBoom) {
			return innerErr
		}

		return nil
	})
	require.NoError(t, err)

	var outerCount, innerCount int64
	require.NoError(t, db.Model(&txRow{}).Where("id = ?", "outer").Count(&outerCount).Error)
	require.NoError(t, db.Model(&txRow{}).Where("id = ?", "inner").Count(&innerCount).Error)
	assert.Equal(t, int64(1), outerCount, "outer row committed")
	assert.Equal(t, int64(0), innerCount, "inner row rolled back by savepoint")
}

func TestDBFrom_ReturnsFallbackWhenNoTxInContext(t *testing.T) {
	t.Parallel()

	db := newTxDB(t)
	got := models.DBFrom(context.Background(), db)
	assert.Same(t, db, got)
}

func TestDBFrom_ReturnsTxFromContext(t *testing.T) {
	t.Parallel()

	db := newTxDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		ctx := models.WithTx(context.Background(), tx)
		got := models.DBFrom(ctx, db)
		assert.Same(t, tx, got, "DBFrom must return the tx attached to ctx, not the fallback")

		return nil
	}))
}
