package health

import (
	"context"
	"testing"

	"github.com/mrz1836/go-foundation/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGORMHealthChecker_Check(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when database is healthy", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		err := hc.Check(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := hc.Check(ctx)
		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

//nolint:gocognit,gocyclo // Test function with multiple sub-tests
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
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if status.Status != StatusHealthy {
			t.Errorf("expected healthy status, got %s", status.Status)
		}

		if status.Version != "1.0.0" {
			t.Errorf("expected version 1.0.0, got %s", status.Version)
		}

		if status.Environment != "test" {
			t.Errorf("expected environment test, got %s", status.Environment)
		}

		if status.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}

		if status.WriteDatabase == nil {
			t.Fatal("expected write database health info")
		}

		if !status.WriteDatabase.Connected {
			t.Error("expected write database connected")
		}

		if status.WriteDatabase.Driver != "sqlite" {
			t.Errorf("expected sqlite driver, got %s", status.WriteDatabase.Driver)
		}

		if status.WriteDatabase.Latency <= 0 {
			t.Error("expected positive latency")
		}
	})

	t.Run("works without config", func(t *testing.T) {
		db := createTestDB(t)
		hc := NewGORMHealthChecker(db, nil)

		status, err := hc.CheckWithDetails(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if status.Status != StatusHealthy {
			t.Errorf("expected healthy status, got %s", status.Status)
		}
		// Version and Environment should be empty without config
		if status.Version != "" {
			t.Errorf("expected empty version without config, got %s", status.Version)
		}
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

		if status == nil {
			t.Error("expected status even when unhealthy")
		}
	})
}

func TestNewGORMHealthChecker(t *testing.T) {
	t.Parallel()

	t.Run("creates health checker", func(t *testing.T) {
		db := createTestDB(t)
		cfg := &config.Config{}

		hc := NewGORMHealthChecker(db, cfg)
		if hc == nil {
			t.Fatal("expected health checker, got nil")
		}

		if hc.writeDB != db {
			t.Error("writeDB not set correctly")
		}

		if hc.cfg != cfg {
			t.Error("cfg not set correctly")
		}
	})
}

// createTestDB creates an in-memory SQLite database for testing.
func createTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	return db
}

func TestGORMHealthChecker_Check_ClosedDatabase(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil)

	// Close the underlying database
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying db: %v", err)
	}

	_ = sqlDB.Close()

	// Check should return an error for closed database
	err = hc.Check(context.Background())
	if err == nil {
		t.Error("expected error for closed database")
	}
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
	if err != nil {
		t.Fatalf("failed to get underlying db: %v", err)
	}

	_ = sqlDB.Close()

	// CheckWithDetails should return unhealthy status for closed database
	status, err := hc.CheckWithDetails(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if status == nil {
		t.Fatal("expected status, got nil")
	}

	if status.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status, got %s", status.Status)
	}

	if status.WriteDatabase == nil {
		t.Fatal("expected write database health info")
	}

	if status.WriteDatabase.Connected {
		t.Error("expected write database not connected for closed db")
	}

	if status.WriteDatabase.Error == "" {
		t.Error("expected error message for closed db")
	}
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
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify all fields are properly populated
	if status.Version != "2.5.0" {
		t.Errorf("expected version 2.5.0, got %s", status.Version)
	}

	if status.Environment != "production" {
		t.Errorf("expected environment production, got %s", status.Environment)
	}

	if status.WriteDatabase.Driver == "" {
		t.Error("expected non-empty driver name")
	}

	if status.WriteDatabase.Latency <= 0 {
		t.Error("expected positive latency measurement")
	}
}

func TestGORMHealthChecker_Check_MultipleSuccessfulCalls(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil)

	// Multiple successful checks should all pass
	for i := range 3 {
		if err := hc.Check(context.Background()); err != nil {
			t.Errorf("check %d failed: %v", i+1, err)
		}
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
		if err != nil {
			t.Errorf("check %d failed: %v", i+1, err)
		}

		if status.Status != StatusHealthy {
			t.Errorf("check %d: expected healthy, got %s", i+1, status.Status)
		}
	}
}
