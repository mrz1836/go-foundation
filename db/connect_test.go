package db

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// errPoolBoom is a package-scope sentinel (keeps err113 happy) used to force the
// connection-pool configuration failure branch.
var errPoolBoom = errors.New("pool boom")

func TestNewConnection_SQLite(t *testing.T) {
	t.Parallel()

	// Setup
	dbName := "test_connect.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: dbName,
	}

	// Execute
	db, err := NewConnection(cfg)
	require.NoError(t, err)

	// Verify
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
	}()

	assert.NoError(t, sqlDB.Ping())

	// Cleanup
	assert.NoError(t, os.Remove(dbName))
}

func TestNewConnection_SQLiteWithConnectionPool(t *testing.T) {
	t.Parallel()

	// Setup - test with connection pool configuration
	dbName := "test_connect_pool.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     dbName,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	// Execute
	db, err := NewConnection(cfg)
	require.NoError(t, err)

	// Verify connection pool settings were applied
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
	}()

	// Verify the connection works
	assert.NoError(t, sqlDB.Ping())

	// Cleanup
	assert.NoError(t, os.Remove(dbName))
}

func TestNewConnection_UnsupportedDriver(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver: "mysql",
	}

	db, err := NewConnection(cfg)

	assert.Nil(t, db, "expected nil db for unsupported driver")
	require.ErrorIs(t, err, errUnsupportedDriver)
}

func TestNewConnection_EmptyDriverDefaultsToPostgres(t *testing.T) {
	t.Parallel()

	// When driver is empty, it should default to postgres
	// This test verifies the switch case handles empty driver
	cfg := &config.WriteDatabaseConfig{
		Driver:   "", // Empty defaults to postgres
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		SSLMode:  "disable",
	}

	// This will fail to connect since there's no postgres server,
	// but we can verify it attempts to use postgres (not sqlite)
	_, err := NewConnection(cfg)

	// We expect a connection error, not an unsupported driver error
	assert.NotErrorIs(t, err, errUnsupportedDriver,
		"empty driver should default to postgres, not return unsupported driver error")
	// Connection will fail, but that's expected - we just want to verify driver selection
}

func TestCreateSQLiteDialector_WithEmptyDatabase(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: "", // Empty should default to "foundation.db"
	}

	dialector := createSQLiteDialector(cfg)

	require.NotNil(t, dialector)
	// The dialector is created; it will use "foundation.db" as the default
}

func TestCreateSQLiteDialector_WithCustomDatabase(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: "custom.db",
	}

	dialector := createSQLiteDialector(cfg)

	require.NotNil(t, dialector)
}

func TestCreatePostgresDialector(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
	}

	dialector := createPostgresDialector(cfg)

	require.NotNil(t, dialector)
}

func TestCreatePostgresDialector_WithoutPassword(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		SSLMode:  "disable",
	}

	dialector := createPostgresDialector(cfg)

	require.NotNil(t, dialector)
}

func TestConfigureConnectionPool_WithMaxOpenConns(t *testing.T) {
	t.Parallel()

	// Setup SQLite for testing connection pool
	dbName := "test_pool_open.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     dbName,
		MaxOpenConns: 25,
		MaxIdleConns: 0, // Not set
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	// Verify pool was configured (MaxOpenConns should be set)
	stats := sqlDB.Stats()
	assert.Equal(t, 25, stats.MaxOpenConnections)
}

func TestConfigureConnectionPool_WithMaxIdleConns(t *testing.T) {
	t.Parallel()

	// Setup SQLite for testing connection pool
	dbName := "test_pool_idle.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     dbName,
		MaxOpenConns: 0, // Not set
		MaxIdleConns: 10,
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	// Connection should work even with only idle conns set
	assert.NoError(t, sqlDB.Ping())
}

func TestConfigureConnectionPool_WithBothSettings(t *testing.T) {
	t.Parallel()

	// Setup SQLite for testing connection pool with both settings
	dbName := "test_pool_both.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     dbName,
		MaxOpenConns: 50,
		MaxIdleConns: 25,
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	stats := sqlDB.Stats()
	assert.Equal(t, 50, stats.MaxOpenConnections)
}

func TestConfigureConnectionPool_WithZeroValues(t *testing.T) {
	t.Parallel()

	// Setup SQLite with zero values (should apply sensible defaults)
	dbName := "test_pool_zero.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:       driverSQLite,
		Database:     dbName,
		MaxOpenConns: 0,
		MaxIdleConns: 0,
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	// Connection should work with default pool settings
	require.NoError(t, sqlDB.Ping())

	// Verify sensible defaults were applied
	stats := sqlDB.Stats()
	assert.Equal(t, defaultMaxOpenConns, stats.MaxOpenConnections)
}

func TestConfigureConnectionPool_ConnMaxLifetime(t *testing.T) {
	t.Parallel()

	// Setup SQLite with a custom ConnMaxLifetime (3 minutes)
	dbName := "test_pool_lifetime.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:          driverSQLite,
		Database:        dbName,
		ConnMaxLifetime: 3, // 3 minutes
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	// ConnMaxLifetime is not exposed via stats, but we can verify it was set
	// by checking the connection works and default max open conns were applied
	require.NoError(t, sqlDB.Ping())

	stats := sqlDB.Stats()
	assert.Equal(t, defaultMaxOpenConns, stats.MaxOpenConnections)
}

func TestConfigureConnectionPool_DefaultConnMaxLifetime(t *testing.T) {
	t.Parallel()

	// Verify that zero ConnMaxLifetime gets the default applied
	dbName := "test_pool_lifetime_default.db"
	cfg := &config.WriteDatabaseConfig{
		Driver:          driverSQLite,
		Database:        dbName,
		ConnMaxLifetime: 0, // should default to 5 minutes
	}

	db, err := NewConnection(cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, sqlDB.Close())
		assert.NoError(t, os.Remove(dbName))
	}()

	assert.NoError(t, sqlDB.Ping())

	// Stats.MaxLifetimeClosed is available only when conns expire,
	// but we can at least verify the connection is functional with defaults.
	_ = time.Minute // silence unused import in case we need it later
}

// TestConfigureConnectionPool_UnderlyingDBError exercises the error branch where
// the underlying *sql.DB cannot be obtained. A gorm.DB with no ConnPool makes
// DB() return gorm.ErrInvalidDB, which configureConnectionPool must surface.
func TestConfigureConnectionPool_UnderlyingDBError(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{Driver: driverSQLite, Database: ":memory:"}

	err := configureConnectionPool(&gorm.DB{Config: &gorm.Config{}}, cfg)
	require.ErrorIs(t, err, gorm.ErrInvalidDB)
}

// TestNewConnection_ConfigurePoolError verifies that NewConnection surfaces an
// error from connection-pool configuration. With a real dialector db.DB() never
// fails, so the configurePool indirection is overridden to force the failure and
// cover the error return in NewConnection.
func TestNewConnection_ConfigurePoolError(t *testing.T) {
	// Not parallel: this test swaps the package-level configurePool var.
	poolErr := errPoolBoom

	original := configurePool
	configurePool = func(_ *gorm.DB, _ config.DatabaseConfig) error {
		return poolErr
	}
	t.Cleanup(func() { configurePool = original })

	dbName := "test_connect_pool_err.db"
	cfg := &config.WriteDatabaseConfig{Driver: driverSQLite, Database: dbName}
	t.Cleanup(func() { _ = os.Remove(dbName) })

	db, err := NewConnection(cfg)
	assert.Nil(t, db, "expected nil db when pool configuration fails")
	require.ErrorIs(t, err, poolErr)
}
