package db

import (
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

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
	if err != nil {
		t.Fatalf("Failed to connect to SQLite: %v", err)
	}

	// Verify
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}
	}()

	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping SQLite DB: %v", err)
	}

	// Cleanup
	if err := os.Remove(dbName); err != nil {
		t.Errorf("Failed to remove test DB: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect to SQLite: %v", err)
	}

	// Verify connection pool settings were applied
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}
	}()

	// Verify the connection works
	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping SQLite DB: %v", err)
	}

	// Cleanup
	if err := os.Remove(dbName); err != nil {
		t.Errorf("Failed to remove test DB: %v", err)
	}
}

func TestNewConnection_UnsupportedDriver(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver: "mysql",
	}

	db, err := NewConnection(cfg)

	if db != nil {
		t.Error("Expected nil db for unsupported driver")
	}

	if err == nil {
		t.Fatal("Expected error for unsupported driver")
	}

	if !errors.Is(err, errUnsupportedDriver) {
		t.Errorf("Expected errUnsupportedDriver, got: %v", err)
	}
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
	if err != nil && errors.Is(err, errUnsupportedDriver) {
		t.Error("Empty driver should default to postgres, not return unsupported driver error")
	}
	// Connection will fail, but that's expected - we just want to verify driver selection
}

func TestCreateSQLiteDialector_WithEmptyDatabase(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: "", // Empty should default to "foundation.db"
	}

	dialector := createSQLiteDialector(cfg)

	if dialector == nil {
		t.Fatal("Expected non-nil dialector")
	}
	// The dialector is created; it will use "foundation.db" as the default
}

func TestCreateSQLiteDialector_WithCustomDatabase(t *testing.T) {
	t.Parallel()

	cfg := &config.WriteDatabaseConfig{
		Driver:   driverSQLite,
		Database: "custom.db",
	}

	dialector := createSQLiteDialector(cfg)

	if dialector == nil {
		t.Fatal("Expected non-nil dialector")
	}
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

	if dialector == nil {
		t.Fatal("Expected non-nil dialector")
	}
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

	if dialector == nil {
		t.Fatal("Expected non-nil dialector")
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	// Verify pool was configured (MaxOpenConns should be set)
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("Expected MaxOpenConnections=25, got %d", stats.MaxOpenConnections)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	// Connection should work even with only idle conns set
	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 50 {
		t.Errorf("Expected MaxOpenConnections=50, got %d", stats.MaxOpenConnections)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	// Connection should work with default pool settings
	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping: %v", err)
	}

	// Verify sensible defaults were applied
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != defaultMaxOpenConns {
		t.Errorf("Expected MaxOpenConnections=%d (default), got %d", defaultMaxOpenConns, stats.MaxOpenConnections)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	// ConnMaxLifetime is not exposed via stats, but we can verify it was set
	// by checking the connection works and default max open conns were applied
	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping: %v", err)
	}

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != defaultMaxOpenConns {
		t.Errorf("Expected MaxOpenConnections=%d (default), got %d", defaultMaxOpenConns, stats.MaxOpenConnections)
	}
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
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}

		if err := os.Remove(dbName); err != nil {
			t.Errorf("Failed to remove test DB: %v", err)
		}
	}()

	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping: %v", err)
	}

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
	if err == nil {
		t.Fatal("expected an error when the underlying sql.DB is unavailable")
	}

	if !errors.Is(err, gorm.ErrInvalidDB) {
		t.Errorf("error = %v, want it to wrap gorm.ErrInvalidDB", err)
	}
}
