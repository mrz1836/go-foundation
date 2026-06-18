// Package testutil provides generic, domain-agnostic test helpers for services
// built on go-foundation: in-memory SQLite databases, ready-made test
// configurations, and (behind the "integration" build tag) a PostgreSQL
// testcontainers harness.
package testutil

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// environmentTest is the conventional environment name used by test configs.
const environmentTest = "test"

// NewTestDB creates an in-memory SQLite database connection for testing.
// The database is created fresh for each test and does not persist.
func NewTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	return db
}

// NewTestConfig creates a generic test configuration backed by SQLite.
// Consuming services can override fields via functional options.
func NewTestConfig(opts ...ConfigOption) *config.Config {
	cfg := &config.Config{
		Environment: environmentTest,
		Application: config.ApplicationConfig{
			Name:        "foundation",
			Version:     environmentTest,
			Environment: environmentTest,
		},
		WriteDatabase: config.WriteDatabaseConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
		ReadDatabase: config.ReadDatabaseConfig{
			Driver:   "sqlite",
			Database: ":memory:",
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// ConfigOption is a functional option for configuring test config.
type ConfigOption func(*config.Config)

// WithEnvironment sets the environment in test config.
func WithEnvironment(env string) ConfigOption {
	return func(c *config.Config) {
		c.Environment = env
		c.Application.Environment = env
	}
}

// WithVersion sets the version in test config.
func WithVersion(version string) ConfigOption {
	return func(c *config.Config) {
		c.Application.Version = version
	}
}
