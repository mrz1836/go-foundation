package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

func TestSearchByNameOpt(t *testing.T) {
	t.Parallel()

	t.Run("non-empty query builds a LIKE condition", func(t *testing.T) {
		t.Parallel()

		sql := builtSQL(t, models.SearchByNameOpt("  Boston  "))
		assert.Contains(t, sql, "LOWER(name) LIKE")
		assert.Contains(t, sql, "Boston")
	})

	t.Run("empty query is a no-op", func(t *testing.T) {
		t.Parallel()
		// The base query still filters soft-deletes; the no-op option must not
		// add a name LIKE clause.
		assert.NotContains(t, builtSQL(t, models.SearchByNameOpt("   ")), "LIKE")
	})
}

func TestFindBySlugOpt(t *testing.T) {
	t.Parallel()

	t.Run("valid slug builds a slug condition", func(t *testing.T) {
		t.Parallel()

		opt, err := models.FindBySlugOpt("My-Slug")
		require.NoError(t, err)

		sql := builtSQL(t, opt)
		assert.Contains(t, sql, "slug =")
		assert.Contains(t, sql, "my-slug", "slug must be normalized to lowercase")
	})

	t.Run("empty slug returns an error", func(t *testing.T) {
		t.Parallel()

		opt, err := models.FindBySlugOpt("   ")
		require.Error(t, err)
		require.ErrorIs(t, err, models.ErrValidation)
		assert.Nil(t, opt)
	})
}

func TestFindNearbyOpt(t *testing.T) {
	t.Parallel()

	t.Run("valid coordinates build a bounding-box condition", func(t *testing.T) {
		t.Parallel()

		opt, err := models.FindNearbyOpt(42.36, -71.05, 5)
		require.NoError(t, err)

		sql := builtSQL(t, opt)
		assert.Contains(t, sql, "latitude BETWEEN")
		assert.Contains(t, sql, "longitude BETWEEN")
	})

	t.Run("invalid latitude returns an error", func(t *testing.T) {
		t.Parallel()

		opt, err := models.FindNearbyOpt(200, 0, 5)
		require.Error(t, err)
		require.ErrorIs(t, err, models.ErrValidation)
		assert.Nil(t, opt)
	})

	t.Run("non-positive radius returns an error", func(t *testing.T) {
		t.Parallel()

		opt, err := models.FindNearbyOpt(42, -71, 0)
		require.Error(t, err)
		require.ErrorIs(t, err, models.ErrValidation)
		assert.Nil(t, opt)
	})
}
