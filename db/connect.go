// Package db provides GORM database connection management for serverless and HTTP services.
package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// Default connection pool settings applied when the config omits them.
const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 5 * time.Minute
)

// Supported database driver identifiers.
const (
	driverSQLite   = "sqlite"
	driverPostgres = "postgres"
)

var errUnsupportedDriver = errors.New("unsupported database driver")

// configurePool indirects configureConnectionPool so tests can inject a failure.
// With a real dialector the underlying db.DB() never errors, leaving the error
// return in NewConnection otherwise unreachable. Overriding this var lets a
// white-box test exercise that branch; production code always uses the default.
var configurePool = configureConnectionPool //nolint:gochecknoglobals // injectable seam for testing the pool-config failure branch

// NewConnection creates a new GORM database connection based on the configuration.
// It supports "sqlite" and "postgres" drivers.
// Accepts any type that implements the DatabaseConfig interface.
func NewConnection(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch cfg.GetDriver() {
	case driverSQLite:
		dialector = createSQLiteDialector(cfg)
	case driverPostgres, "": // Default to postgres
		dialector = createPostgresDialector(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedDriver, cfg.GetDriver())
	}

	// Open connection
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	if err := configurePool(db, cfg); err != nil {
		return nil, err
	}

	return db, nil
}

// createSQLiteDialector creates a SQLite dialector with the given configuration.
func createSQLiteDialector(cfg config.DatabaseConfig) gorm.Dialector {
	// For SQLite, we use the Database field as the file path.
	// If it's empty, we default to "foundation.db"
	dbName := cfg.GetDatabase()
	if dbName == "" {
		dbName = "foundation.db"
	}

	return sqlite.Open(dbName)
}

// createPostgresDialector creates a Postgres dialector with the given configuration.
func createPostgresDialector(cfg config.DatabaseConfig) gorm.Dialector {
	dsn := cfg.ConnectionString()
	return postgres.Open(dsn)
}

// configureConnectionPool configures the connection pool settings for the database.
// Sensible defaults are applied when the config values are zero.
func configureConnectionPool(db *gorm.DB, cfg config.DatabaseConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	maxOpen := cfg.GetMaxOpenConns()
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpenConns
	}

	sqlDB.SetMaxOpenConns(maxOpen)

	maxIdle := cfg.GetMaxIdleConns()
	if maxIdle <= 0 {
		maxIdle = defaultMaxIdleConns
	}

	sqlDB.SetMaxIdleConns(maxIdle)

	lifetime := time.Duration(cfg.GetConnMaxLifetime()) * time.Minute
	if lifetime <= 0 {
		lifetime = defaultConnMaxLifetime
	}

	sqlDB.SetConnMaxLifetime(lifetime)

	return nil
}
