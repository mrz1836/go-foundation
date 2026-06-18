//go:build integration

package testutil_test

import (
	"testing"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// widgetID is a typed string ID, mirroring how consuming bounded contexts
// declare their own ID types over models.BaseModel.
type widgetID string

// widget is a minimal model used to exercise the generic Repository against a
// real PostgreSQL backend.
type widget struct {
	models.BaseModel[widgetID]

	Name string `gorm:"size:100;not null"`
}

// TestRepository_PostgresRoundTrip validates that the generic Repository, the
// UUID v7 BeforeCreate hook, and timestamp handling all work against PostgreSQL
// (the production driver), complementing the SQLite-based unit tests.
func TestRepository_PostgresRoundTrip(t *testing.T) {
	db := testutil.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&widget{}))

	repo := models.NewRepository[widget, widgetID](db)
	ctx := t.Context()

	w := &widget{Name: "first"}
	require.NoError(t, repo.Create(ctx, w))
	assert.NotEmpty(t, w.ID, "BeforeCreate should generate a UUID v7")
	assert.False(t, w.CreatedAt.IsZero(), "CreatedAt should be populated")

	got, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Name)

	got.Name = "second"
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.Find(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "second", reloaded.Name)

	require.NoError(t, repo.Delete(ctx, w.ID))
	_, err = repo.Find(ctx, w.ID)
	require.ErrorIs(t, err, models.ErrNotFound, "soft-deleted record should not be found")
}
