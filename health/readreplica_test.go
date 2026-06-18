package health

import (
	"context"
	"strings"
	"testing"

	"github.com/mrz1836/go-foundation/config"
)

func TestGORMHealthChecker_CheckWithDetails_SharedReadConnection(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	hc := NewGORMHealthChecker(db, nil, WithReadDatabase(db)) // same handle for read + write

	status, err := hc.CheckWithDetails(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Status != StatusHealthy {
		t.Errorf("status = %s, want healthy", status.Status)
	}

	if status.ReadDatabase == nil {
		t.Fatal("expected read database health for a shared connection")
	}

	if !strings.Contains(status.ReadDatabase.Host, "shared with write") {
		t.Errorf("read host = %q, want it to note the shared connection", status.ReadDatabase.Host)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Status != StatusHealthy {
		t.Errorf("status = %s, want healthy", status.Status)
	}

	if status.ReadDatabase == nil || !status.ReadDatabase.Connected {
		t.Fatal("expected a connected, separate read database")
	}

	if status.ReadDatabase.Host != "reader.example.com" {
		t.Errorf("read host = %q, want reader.example.com", status.ReadDatabase.Host)
	}

	if status.WriteDatabase.Host != "writer.example.com" {
		t.Errorf("write host = %q, want writer.example.com", status.WriteDatabase.Host)
	}
}

func TestGORMHealthChecker_CheckWithDetails_UnhealthyReadIsDegraded(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)
	hc := NewGORMHealthChecker(writeDB, nil, WithReadDatabase(readDB))

	// Close only the read replica: write stays healthy, so overall = degraded.
	readSQL, err := readDB.DB()
	if err != nil {
		t.Fatalf("get read db: %v", err)
	}

	_ = readSQL.Close()

	status, err := hc.CheckWithDetails(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Status != StatusDegraded {
		t.Errorf("status = %s, want degraded", status.Status)
	}

	if status.ReadDatabase == nil || status.ReadDatabase.Connected {
		t.Error("expected the read database to report as not connected")
	}
}

func TestGORMHealthChecker_SetReadDatabase(t *testing.T) {
	t.Parallel()

	writeDB := createTestDB(t)
	readDB := createTestDB(t)

	hc := NewGORMHealthChecker(writeDB, nil)
	hc.SetReadDatabase(readDB)

	if hc.readDB != readDB {
		t.Fatal("SetReadDatabase did not attach the read connection")
	}

	if err := hc.Check(context.Background()); err != nil {
		t.Errorf("unexpected error with healthy read replica: %v", err)
	}
}

func TestGORMHealthChecker_NilWriteDatabase(t *testing.T) {
	t.Parallel()

	hc := NewGORMHealthChecker(nil, nil)

	// Check pings nil → ErrDatabaseNotConfigured.
	if err := hc.Check(context.Background()); err == nil {
		t.Error("expected an error when the write database is nil")
	}

	// CheckWithDetails reports the write database as unconfigured/unhealthy.
	status, err := hc.CheckWithDetails(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Status != StatusUnhealthy {
		t.Errorf("status = %s, want unhealthy", status.Status)
	}

	if status.WriteDatabase == nil || status.WriteDatabase.Connected {
		t.Error("expected write database to report as not connected")
	}

	if status.WriteDatabase.Error == "" {
		t.Error("expected an error message for the unconfigured write database")
	}
}
