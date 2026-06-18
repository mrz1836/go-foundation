package testutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/testutil"
)

func TestNewTestDB(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	require.NotNil(t, db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
}

func TestNewTestConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig()
	assert.Equal(t, "test", cfg.Environment)
	assert.Equal(t, "foundation", cfg.Application.Name)
	assert.Equal(t, "sqlite", cfg.WriteDatabase.Driver)
	assert.Equal(t, ":memory:", cfg.WriteDatabase.Database)
	assert.Equal(t, ":memory:", cfg.ReadDatabase.Database)
}

func TestNewTestConfig_Options(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(
		testutil.WithEnvironment("staging"),
		testutil.WithVersion("v1.2.3"),
	)
	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, "staging", cfg.Application.Environment)
	assert.Equal(t, "v1.2.3", cfg.Application.Version)
}
