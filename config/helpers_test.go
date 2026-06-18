package config_test

import (
	"github.com/mrz1836/go-foundation/config"
)

// newValidConfig returns a minimal valid Config for testing.
func newValidConfig() config.Config {
	return config.Config{
		Environment: "development",
		Application: config.ApplicationConfig{
			Name:    "test-app",
			Version: "1.0.0",
			Timeout: 30,
		},
		WriteDatabase: config.WriteDatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "app_test",
			Username:        "testuser",
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5,
			Driver:          "pgx",
		},
		ReadDatabase: config.ReadDatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "app_test",
			Username:        "testuser",
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5,
			Driver:          "pgx",
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		AWS: config.AWSConfig{
			Region: "us-east-1",
		},
	}
}
