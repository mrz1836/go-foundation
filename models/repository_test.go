package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// widgetID is a typed string ID over models.BaseModel.
type widgetID string

// errRollback is a static sentinel used to trigger transaction rollback in tests.
var errRollback = errors.New("boom")

// widget is a minimal model used to exercise the generic Repository on SQLite.
type widget struct {
	models.BaseModel[widgetID]

	Name string `gorm:"size:100;uniqueIndex;not null"`
	Tag  string `gorm:"size:50"`
}

func newRepoDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&widget{}))

	return db
}

func TestRepository_CRUDLifecycle(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()

	w := &widget{Name: "alpha", Tag: "x"}
	require.NoError(t, repo.Create(ctx, w))
	assert.NotEmpty(t, w.ID, "BeforeCreate populates a UUID v7")
	assert.False(t, w.CreatedAt.IsZero())

	got, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "alpha", got.Name)

	got.Tag = "y"
	require.NoError(t, repo.Update(ctx, got))
	reloaded, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "y", reloaded.Tag)

	exists, err := repo.Exists(ctx, w.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, repo.Delete(ctx, w.ID))
	_, err = repo.Find(ctx, w.ID)
	require.ErrorIs(t, err, models.ErrNotFound)

	// Soft-deleted rows are visible with WithIncludeDeleted.
	all, err := repo.FindAll(ctx, models.WithIncludeDeleted(true))
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, repo.Restore(ctx, w.ID))
	restored, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "alpha", restored.Name)

	require.NoError(t, repo.HardDelete(ctx, w.ID))
	all, err = repo.FindAll(ctx, models.WithIncludeDeleted(true))
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestRepository_QueryOptions(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, repo.Create(ctx, &widget{Name: name, Tag: "grp"}))
	}

	count, err := repo.Count(ctx, models.WithCondition("tag = ?", "grp"))
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	limited, err := repo.FindAll(
		ctx,
		models.WithOrderBy("name", true),
		models.WithLimit(2),
		models.WithOffset(0),
		models.WithSelect("id", "name"),
	)
	require.NoError(t, err)
	require.Len(t, limited, 2)
	assert.Equal(t, "c", limited[0].Name, "descending order by name")

	one, err := repo.FindOne(ctx, models.WithCondition("name = ?", "b"))
	require.NoError(t, err)
	assert.Equal(t, "b", one.Name)
}

func TestRepository_InvalidID(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()

	_, err := repo.Find(ctx, "not-a-uuid")
	require.ErrorIs(t, err, models.ErrInvalidID)

	require.ErrorIs(t, repo.Delete(ctx, ""), models.ErrInvalidID)
	require.ErrorIs(t, repo.HardDelete(ctx, "bad"), models.ErrInvalidID)
	require.ErrorIs(t, repo.Restore(ctx, "bad"), models.ErrInvalidID)

	_, err = repo.Exists(ctx, "bad")
	require.ErrorIs(t, err, models.ErrInvalidID)
}

func TestRepository_DuplicateKeyMapsToTypedError(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &widget{Name: "dup"}))
	err := repo.Create(ctx, &widget{Name: "dup"})
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrDuplicateKey, "unique violation maps to ErrDuplicateKey")
}

func TestRepository_NotFoundOnMissing(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()

	missing := widgetID(models.NewID())
	require.ErrorIs(t, repo.Delete(ctx, missing), models.ErrNotFound)
	require.ErrorIs(t, repo.HardDelete(ctx, missing), models.ErrNotFound)
	require.ErrorIs(t, repo.Restore(ctx, missing), models.ErrNotFound)

	exists, err := repo.Exists(ctx, missing)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRepository_DBHandle(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	assert.Same(t, db, repo.DB())
}

func TestTransactor_CommitAndRollback(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	repo := models.NewRepository[widget, widgetID](db)
	tx := models.NewTransactor(db)
	ctx := context.Background()

	// Commit: repository calls inside WithinTx pick up the tx via DBFrom.
	require.NoError(t, tx.WithinTx(ctx, func(ctx context.Context) error {
		return repo.Create(ctx, &widget{Name: "committed"})
	}))
	exists, err := repo.Count(ctx, models.WithCondition("name = ?", "committed"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	// Rollback: returning an error rolls the whole unit of work back.
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if cErr := repo.Create(ctx, &widget{Name: "rolled-back"}); cErr != nil {
			return cErr
		}

		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	count, err := repo.Count(ctx, models.WithCondition("name = ?", "rolled-back"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "rolled-back row must not persist")
}

func TestDBFrom_FallsBackWithoutTx(t *testing.T) {
	t.Parallel()
	db := newRepoDB(t)
	assert.Same(t, db, models.DBFrom(context.Background(), db))

	ctx := models.WithTx(context.Background(), db)
	assert.Same(t, db, models.DBFrom(ctx, nil))
}

func TestRepository_ReadWriteRouting(t *testing.T) {
	t.Parallel()
	writeDB := newRepoDB(t)
	readDB := newRepoDB(t)
	repo := models.NewRepositoryWithReadWrite[widget, widgetID](writeDB, readDB)
	ctx := context.Background()

	assert.Same(t, writeDB, repo.DB())
	assert.Same(t, readDB, repo.ReadDB())

	// Writes target the primary only.
	w := &widget{Name: "primary-only"}
	require.NoError(t, repo.Create(ctx, w))
	require.NoError(t, writeDB.First(&widget{}, "id = ?", string(w.ID)).Error)
	require.ErrorIs(t, readDB.First(&widget{}, "id = ?", string(w.ID)).Error, gorm.ErrRecordNotFound)

	// Reads target the replica: the primary-only row is invisible to repo reads.
	_, err := repo.Find(ctx, w.ID)
	require.ErrorIs(t, err, models.ErrNotFound)

	// A row that exists only on the replica IS visible to repo reads.
	replicaRow := &widget{Name: "replica-only"}
	require.NoError(t, readDB.Create(replicaRow).Error)

	got, err := repo.Find(ctx, replicaRow.ID)
	require.NoError(t, err)
	assert.Equal(t, "replica-only", got.Name)
}

func TestRepository_ReadsInsideTxUsePrimary(t *testing.T) {
	t.Parallel()
	writeDB := newRepoDB(t)
	readDB := newRepoDB(t)
	repo := models.NewRepositoryWithReadWrite[widget, widgetID](writeDB, readDB)
	tx := models.NewTransactor(writeDB)

	// Inside a transaction, a read must observe the uncommitted write — i.e. it
	// must use the transaction (primary), not the read replica.
	require.NoError(t, tx.WithinTx(context.Background(), func(ctx context.Context) error {
		w := &widget{Name: "in-tx"}
		if err := repo.Create(ctx, w); err != nil {
			return err
		}

		got, err := repo.Find(ctx, w.ID)
		if err != nil {
			return err
		}

		assert.Equal(t, "in-tx", got.Name)

		return nil
	}))
}

func TestNewRepositoryWithReadWrite_NilReadFallsBackToWrite(t *testing.T) {
	t.Parallel()
	writeDB := newRepoDB(t)
	repo := models.NewRepositoryWithReadWrite[widget, widgetID](writeDB, nil)
	ctx := context.Background()

	assert.Same(t, writeDB, repo.DB())
	assert.Same(t, writeDB, repo.ReadDB())

	// With no replica, reads observe writes immediately (single connection).
	w := &widget{Name: "single-db"}
	require.NoError(t, repo.Create(ctx, w))

	got, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "single-db", got.Name)
}
