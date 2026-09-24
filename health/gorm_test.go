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

func TestGORMHealthChecker_Check(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when database is healthy", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		assert.NoError(t, hc.Check(context.Background()))
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		assert.Error(t, hc.Check(ctx), "expected error for canceled context")
	})
}

func TestGORMHealthChecker_CheckWithDetails(t *testing.T) {
	t.Parallel()

	t.Run("returns healthy status with details", func(t *testing.T) {
		db := createTestDB(t)
		cfg := &config.Config{
			Application: config.ApplicationConfig{
				Environment: "test",
				Version:     "1.0.0",
				Commit:      "abc1234",
			},
		}
		hc := NewGORMHealthChecker(db, cfg)

		status, err := hc.CheckWithDetails(context.Background())
		require.NoError(t, err)

		assert.Equal(t, StatusHealthy, status.Status)
		assert.Equal(t, "1.0.0", status.Version)
		assert.Equal(t, "test", status.Environment)
		assert.False(t, status.Timestamp.IsZero(), "expected non-zero timestamp")

		require.NotNil(t, status.WriteDatabase, "expected write database health info")
		assert.True(t, status.WriteDatabase.Connected, "expected write database connected")
		assert.Equal(t, "sqlite", status.WriteDatabase.Driver)
		assert.Positive(t, status.WriteDatabase.Latency, "expected positive latency")
	})

	t.Run("works without config", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		status, err := hc.CheckWithDetails(context.Background())
		require.NoError(t, err)

		assert.Equal(t, StatusHealthy, status.Status)
		// Version and Environment should be empty without config
		assert.Empty(t, status.Version, "expected empty version without config")
	})

	t.Run("respects context timeout", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		<-ctx.Done() // deterministic; no sleep needed

		status, err := hc.CheckWithDetails(ctx)
		// Should return status even with error (unhealthy)
		if err != nil {
			t.Logf("got error as expected: %v", err)
		}

		assert.NotNil(t, status, "expected status even when unhealthy")
	})
}

func TestNewGORMHealthChecker(t *testing.T) {
	t.Parallel()

	t.Run("creates health checker", func(t *testing.T) {
		db := createTestDB(t)
		cfg := &config.Config{}

		hc := NewGORMHealthChecker(db, cfg)
		require.NotNil(t, hc, "expected health checker, got nil")

		assert.Same(t, db, hc.writeDB, "writeDB not set correctly")
		assert.Same(t, cfg, hc.cfg, "cfg not set correctly")
		assert.Nil(t, hc.readDB, "readDB should be nil without WithReadDatabase")
	})

	t.Run("WithReadDatabase attaches a read connection", func(t *testing.T) {
		writeDB := createTestDB(t)
		readDB := createTestDB(t)

		hc := NewGORMHealthChecker(writeDB, nil, WithReadDatabase(readDB))
		assert.Same(t, readDB, hc.readDB, "readDB not set by WithReadDatabase")
		assert.Same(t, writeDB, hc.writeDB, "writeDB not set correctly")
	})
}

func TestGORMHealthChecker_Check_WithReadReplica(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)
	hc := NewGORMHealthChecker(writeDB, nil, WithReadDatabase(readDB))

	require.NoError(t, hc.Check(context.Background()), "unexpected error with healthy read replica")

	// Closing the read replica must surface as an unhealthy check.
	sqlDB, err := readDB.DB()
	require.NoError(t, err, "failed to get underlying read db")

	_ = sqlDB.Close()

	assert.Error(t, hc.Check(context.Background()), "expected error when read replica is down")
}

// createTestDB creates an in-memory SQLite database for testing.
func createTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to create test db")

	return db
}

func TestGORMHealthChecker_Check_ClosedDatabase(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil)

	// Close the underlying database
	sqlDB, err := db.DB()
	require.NoError(t, err, "failed to get underlying db")

	_ = sqlDB.Close()

	// Check should return an error for closed database
	assert.Error(t, hc.Check(context.Background()), "expected error for closed database")
}

func TestGORMHealthChecker_CheckWithDetails_ClosedDatabase(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	cfg := &config.Config{
		Application: config.ApplicationConfig{
			Environment: "test",
			Version:     "1.0.0",
			Commit:      "abc1234",
		},
	}
	hc := NewGORMHealthChecker(db, cfg)

	// Close the underlying database
	sqlDB, err := db.DB()
	require.NoError(t, err, "failed to get underlying db")

	_ = sqlDB.Close()

	// CheckWithDetails should return unhealthy status for closed database
	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)
	require.NotNil(t, status, "expected status, got nil")

	assert.Equal(t, StatusUnhealthy, status.Status)

	require.NotNil(t, status.WriteDatabase, "expected write database health info")
	assert.False(t, status.WriteDatabase.Connected, "expected write database not connected for closed db")
	assert.NotEmpty(t, status.WriteDatabase.Error, "expected error message for closed db")
}

func TestGORMHealthChecker_CheckWithDetails_VerifyFields(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	cfg := &config.Config{
		Application: config.ApplicationConfig{
			Environment: "production",
			Version:     "2.5.0",
		},
	}
	hc := NewGORMHealthChecker(db, cfg)

	status, err := hc.CheckWithDetails(context.Background())
	require.NoError(t, err)

	// Verify all fields are properly populated
	assert.Equal(t, "2.5.0", status.Version)
	assert.Equal(t, "production", status.Environment)
	assert.NotEmpty(t, status.WriteDatabase.Driver, "expected non-empty driver name")
	assert.Positive(t, status.WriteDatabase.Latency, "expected positive latency measurement")
}

func TestGORMHealthChecker_Check_MultipleSuccessfulCalls(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil)

	// Multiple successful checks should all pass
	for i := range 3 {
		assert.NoErrorf(t, hc.Check(context.Background()), "check %d failed", i+1)
	}
}

func TestGORMHealthChecker_CheckWithDetails_MultipleSuccessfulCalls(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	cfg := &config.Config{
		Application: config.ApplicationConfig{
			Version: "1.0.0",
			Commit:  "abc1234",
		},
	}
	hc := NewGORMHealthChecker(db, cfg)

	// Multiple successful checks should all return healthy
	for i := range 3 {
		status, err := hc.CheckWithDetails(context.Background())
		require.NoErrorf(t, err, "check %d failed", i+1)

		assert.Equalf(t, StatusHealthy, status.Status, "check %d: expected healthy", i+1)
	}
}

// TestGORMHealthChecker_UnderlyingDBError covers the branches where the
// underlying *sql.DB cannot be retrieved. A gorm.DB with no ConnPool makes
// DB() fail, which both pingDatabase and checkDatabaseHealth must report.
func TestGORMHealthChecker_UnderlyingDBError(t *testing.T) {
	t.Parallel()

	broken := &gorm.DB{Config: &gorm.Config{}}
	hc := NewGORMHealthChecker(broken, nil)
	ctx := context.Background()

	require.Error(t, hc.pingDatabase(ctx, broken, "write"), "pingDatabase: expected an error when DB() fails")

	dbHealth := hc.checkDatabaseHealth(ctx, broken, "write")
	assert.False(t, dbHealth.Connected, "checkDatabaseHealth: expected Connected=false when DB() fails")
	assert.NotEmpty(t, dbHealth.Error, "checkDatabaseHealth: expected a non-empty error message")
}
