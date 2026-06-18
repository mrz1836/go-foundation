package models_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// optChild is a trivial association used to exercise WithPreload.
type optChild struct {
	ID       string `gorm:"primaryKey"`
	OptRowID string
}

// optRow is a throwaway model whose generated SQL we inspect to assert the
// effect of each QueryOption. It carries a soft-delete field (to observe
// WithIncludeDeleted) and a has-many association (to observe WithPreload).
type optRow struct {
	ID         string `gorm:"primaryKey"`
	Name       string
	Population int
	Children   []optChild `gorm:"foreignKey:OptRowID"`
	DeletedAt  gorm.DeletedAt
}

// newOptionTestDB opens an in-memory SQLite database with the option models
// migrated, suitable for DryRun SQL generation.
func newOptionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open sqlite test db")
	require.NoError(t, db.AutoMigrate(&optRow{}, &optChild{}), "migrate option models")

	return db
}

// builtSQL renders the SQL produced by applying opts to a Find against optRow.
func builtSQL(t *testing.T, opts ...models.QueryOption) string {
	t.Helper()

	db := newOptionTestDB(t)

	return db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var dest []optRow

		return models.ApplyOptions(tx.Model(&optRow{}), opts...).Find(&dest)
	})
}

func TestWithLimit(t *testing.T) {
	t.Parallel()

	t.Run("positive limit applied", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, builtSQL(t, models.WithLimit(10)), "LIMIT 10")
	})

	t.Run("zero limit is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithLimit(0)), "LIMIT")
	})

	t.Run("negative limit is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithLimit(-5)), "LIMIT")
	})
}

func TestWithOffset(t *testing.T) {
	t.Parallel()

	t.Run("positive offset applied", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, builtSQL(t, models.WithOffset(20)), "OFFSET 20")
	})

	t.Run("zero offset is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithOffset(0)), "OFFSET")
	})

	t.Run("negative offset is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithOffset(-1)), "OFFSET")
	})
}

func TestWithIncludeDeleted(t *testing.T) {
	t.Parallel()

	t.Run("default query filters soft-deleted rows", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, builtSQL(t), "deleted_at")
	})

	t.Run("include true unscopes the soft-delete filter", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithIncludeDeleted(true)), "deleted_at")
	})

	t.Run("include false keeps the soft-delete filter", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, builtSQL(t, models.WithIncludeDeleted(false)), "deleted_at")
	})
}

func TestWithSelect(t *testing.T) {
	t.Parallel()

	t.Run("named fields applied", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t, models.WithSelect("id", "name"))
		assert.Contains(t, sql, "id")
		assert.Contains(t, sql, "name")
		assert.NotContains(t, sql, "SELECT *")
	})

	t.Run("empty select is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, builtSQL(t, models.WithSelect()), "SELECT *")
	})
}

func TestWithOrderBy(t *testing.T) {
	t.Parallel()

	t.Run("ascending", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t, models.WithOrderBy("name", false))
		assert.Contains(t, sql, "ORDER BY")
		assert.Contains(t, sql, "ASC")
	})

	t.Run("descending", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t, models.WithOrderBy("created_at", true))
		assert.Contains(t, sql, "ORDER BY")
		assert.Contains(t, sql, "DESC")
	})

	t.Run("empty field is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, builtSQL(t, models.WithOrderBy("", false)), "ORDER BY")
	})
}

func TestWithCondition(t *testing.T) {
	t.Parallel()

	sql := builtSQL(t, models.WithCondition("population > ?", 1000000))
	assert.Contains(t, sql, "population >")
	assert.Contains(t, sql, "1000000")
}

func TestWithPreload(t *testing.T) {
	t.Parallel()

	db := newOptionTestDB(t)

	var dest []optRow

	tx := models.ApplyOptions(
		db.Session(&gorm.Session{DryRun: true}).Model(&optRow{}),
		models.WithPreload("Children"),
	).Find(&dest)

	require.NoError(t, tx.Error)
	assert.Contains(t, tx.Statement.Preloads, "Children", "preload must be registered")
}

func TestApplyOptions_Composition(t *testing.T) {
	t.Parallel()

	t.Run("no options returns an unmodified query", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t)
		assert.Contains(t, sql, "SELECT")
		assert.NotContains(t, sql, "LIMIT")
		assert.NotContains(t, sql, "ORDER BY")
	})

	t.Run("multiple options all apply", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t,
			models.WithCondition("population > ?", 100),
			models.WithOrderBy("name", false),
			models.WithLimit(5),
			models.WithOffset(10),
		)

		assert.Contains(t, sql, "population >")
		assert.Contains(t, sql, "ORDER BY")
		assert.Contains(t, sql, "LIMIT 5")
		assert.Contains(t, sql, "OFFSET 10")
	})

	t.Run("ordering of WHERE clauses is preserved", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t,
			models.WithCondition("a = ?", 1),
			models.WithCondition("b = ?", 2),
		)
		assert.Less(t, strings.Index(sql, "a ="), strings.Index(sql, "b ="),
			"conditions must apply in the order supplied")
	})
}
