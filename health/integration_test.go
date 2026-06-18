package health

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// TestHealthChecker_SQLiteIntegration demonstrates that the health service
// works identically with SQLite as with PostgreSQL.
//
//nolint:gocognit,gocyclo // Integration test with multiple assertions
func TestHealthChecker_SQLiteIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create SQLite in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create SQLite db: %v", err)
	}

	cfg := &config.Config{
		Application: config.ApplicationConfig{
			Environment: "test",
			Version:     "1.0.0-test",
		},
	}

	// Create health checker with SQLite
	hc := NewGORMHealthChecker(db, cfg)

	t.Run("Check returns nil for healthy SQLite", func(t *testing.T) {
		err := hc.Check(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("CheckWithDetails returns correct driver", func(t *testing.T) {
		status, err := hc.CheckWithDetails(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if status.Status != StatusHealthy {
			t.Errorf("expected healthy status, got %s", status.Status)
		}

		if status.WriteDatabase == nil {
			t.Fatal("expected write database info")
		}

		if status.WriteDatabase.Driver != "sqlite" {
			t.Errorf("expected sqlite driver, got %s", status.WriteDatabase.Driver)
		}

		if !status.WriteDatabase.Connected {
			t.Error("expected write database to be connected")
		}

		if status.Version != "1.0.0-test" {
			t.Errorf("expected version 1.0.0-test, got %s", status.Version)
		}

		if status.Environment != "test" {
			t.Errorf("expected environment test, got %s", status.Environment)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify structure matches what PostgreSQL would return
	if status.Status == "" {
		t.Error("status should not be empty")
	}

	if status.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}

	if status.WriteDatabase == nil {
		t.Fatal("write database info should be present")
	}

	if status.WriteDatabase.Latency <= 0 {
		t.Error("latency should be positive")
	}
	// Driver will be different (sqlite vs postgres), but structure is same
}
