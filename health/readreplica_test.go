package health

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/config"
)

func TestGORMHealthChecker_CheckWithDetails_SharedReadConnection(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil, WithReadDatabase(db)) // same handle for read + write

	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	assert.Equal(t, StatusHealthy, status.Status)

	require.NotNil(t, status.ReadDatabase, "expected read database health for a shared connection")
	assert.Contains(t, status.ReadDatabase.Host, "shared with write", "read host should note the shared connection")
}

func TestGORMHealthChecker_CheckWithDetails_SeparateHealthyRead(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)
	cfg := &config.Config{}
	cfg.WriteDatabase.Host = "writer.example.com"
	cfg.ReadDatabase.Host = "reader.example.com"

	hc := NewGORMHealthChecker(writeDB, cfg, WithReadDatabase(readDB))

	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	assert.Equal(t, StatusHealthy, status.Status)

	require.NotNil(t, status.ReadDatabase, "expected a separate read database")
	assert.True(t, status.ReadDatabase.Connected, "expected the read database to be connected")
	assert.Equal(t, "reader.example.com", status.ReadDatabase.Host)
	assert.Equal(t, "writer.example.com", status.WriteDatabase.Host)
}

func TestGORMHealthChecker_CheckWithDetails_UnhealthyReadIsDegraded(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)
	hc := NewGORMHealthChecker(writeDB, nil, WithReadDatabase(readDB))

	// Close only the read replica: write stays healthy, so overall = degraded.
	readSQL, err := readDB.DB()
	require.NoError(t, err, "get read db")

	_ = readSQL.Close()

	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	assert.Equal(t, StatusDegraded, status.Status)

	require.NotNil(t, status.ReadDatabase)
	assert.False(t, status.ReadDatabase.Connected, "expected the read database to report as not connected")
}

func TestGORMHealthChecker_SetReadDatabase(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)

	hc := NewGORMHealthChecker(writeDB, nil)
	hc.SetReadDatabase(readDB)

	require.Same(t, readDB, hc.readDB, "SetReadDatabase did not attach the read connection")

	assert.NoError(t, hc.Check(context.Background()), "unexpected error with healthy read replica")
}

func TestGORMHealthChecker_NilWriteDatabase(t *testing.T) {
	t.Parallel()

	hc := NewGORMHealthChecker(nil, nil)

	// Check pings nil → ErrDatabaseNotConfigured.
	require.Error(t, hc.Check(context.Background()), "expected an error when the write database is nil")

	// CheckWithDetails reports the write database as unconfigured/unhealthy.
	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	assert.Equal(t, StatusUnhealthy, status.Status)

	require.NotNil(t, status.WriteDatabase)
	assert.False(t, status.WriteDatabase.Connected, "expected write database to report as not connected")
	assert.NotEmpty(t, status.WriteDatabase.Error, "expected an error message for the unconfigured write database")
}
