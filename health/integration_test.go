package health

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// TestHealthChecker_SQLiteIntegration demonstrates that the health service
// works identically with SQLite as with PostgreSQL.
func TestHealthChecker_SQLiteIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create SQLite in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to create SQLite db")

	cfg := &config.Config{
		Application: config.ApplicationConfig{
			Environment: "test",
			Version:     "1.0.0-test",
		},
	}

	// Create health checker with SQLite
	hc := NewGORMHealthChecker(db, cfg)

	t.Run("Check returns nil for healthy SQLite", func(t *testing.T) {
		assert.NoError(t, hc.Check(context.Background()))
	})

	t.Run("CheckWithDetails returns correct driver", func(t *testing.T) {
		status, err := hc.CheckWithDetails(context.Background())
		require.NoError(t, err)

		assert.Equal(t, StatusHealthy, status.Status)

		require.NotNil(t, status.WriteDatabase, "expected write database info")
		assert.Equal(t, "sqlite", status.WriteDatabase.Driver)
		assert.True(t, status.WriteDatabase.Connected, "expected write database to be connected")
		assert.Equal(t, "1.0.0-test", status.Version)
		assert.Equal(t, "test", status.Environment)
	})
}

// TestHealthChecker_SQLiteSameAsPostgres documents that SQLite health checks
// behave the same as PostgreSQL for testing purposes.
func TestHealthChecker_SQLiteSameAsPostgres(t *testing.T) {
	// This test verifies the contract: SQLite health checks should return
	// the same structure as PostgreSQL, enabling:
	// 1. Fast local testing without PostgreSQL
	// 2. CI/CD pipelines without database infrastructure
	// 3. Unit tests that are independent of database type
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	hc := NewGORMHealthChecker(db, nil)

	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	// Verify structure matches what PostgreSQL would return
	assert.NotEmpty(t, status.Status, "status should not be empty")
	assert.False(t, status.Timestamp.IsZero(), "timestamp should be set")

	require.NotNil(t, status.WriteDatabase, "write database info should be present")
	assert.Positive(t, status.WriteDatabase.Latency, "latency should be positive")
	// Driver will be different (sqlite vs postgres), but structure is same
}
