package health

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/config"
)

// GORMHealthChecker is the GORM-based implementation of HealthChecker.
type GORMHealthChecker struct {
	writeDB *gorm.DB
	readDB  *gorm.DB
	cfg     *config.Config
}

// NewGORMHealthChecker creates a new GORM-based health checker.
// It accepts both write and read database connections.
// If readDB is nil, only the write database will be checked.
func NewGORMHealthChecker(writeDB *gorm.DB, cfg *config.Config) *GORMHealthChecker {
	return &GORMHealthChecker{
		writeDB: writeDB,
		readDB:  nil, // Set via SetReadDatabase
		cfg:     cfg,
	}
}

// NewGORMHealthCheckerWithReadWrite creates a new GORM-based health checker with both connections.
func NewGORMHealthCheckerWithReadWrite(writeDB, readDB *gorm.DB, cfg *config.Config) *GORMHealthChecker {
	return &GORMHealthChecker{
		writeDB: writeDB,
		readDB:  readDB,
		cfg:     cfg,
	}
}

// SetReadDatabase sets the read database connection for health checking.
func (h *GORMHealthChecker) SetReadDatabase(db *gorm.DB) {
	h.readDB = db
}

// Check performs a basic health check by pinging both databases.
// Returns an error if either database is unhealthy.
func (h *GORMHealthChecker) Check(ctx context.Context) error {
	// Check write database
	if err := h.pingDatabase(ctx, h.writeDB, "write"); err != nil {
		return err
	}

	// Check read database if configured and different from write
	if h.readDB != nil && h.readDB != h.writeDB {
		if err := h.pingDatabase(ctx, h.readDB, "read"); err != nil {
			return err
		}
	}

	return nil
}

// CheckWithDetails performs a detailed health check with timing and diagnostic info.
func (h *GORMHealthChecker) CheckWithDetails(ctx context.Context) (*Status, error) {
	status := &Status{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
	}

	// Add config info if available
	if h.cfg != nil {
		status.Version = h.cfg.Application.Version
		status.Environment = h.cfg.Application.Environment
	}

	// Check write database
	writeHealth := h.checkDatabaseHealth(ctx, h.writeDB, "write")

	status.WriteDatabase = writeHealth
	if !writeHealth.Connected {
		status.Status = StatusUnhealthy
	}

	// Handle read database health based on configuration
	status.ReadDatabase = h.getReadDatabaseHealth(ctx, writeHealth, status)

	return status, nil
}

// getReadDatabaseHealth returns health info for the read database, updating status if needed.
func (h *GORMHealthChecker) getReadDatabaseHealth(ctx context.Context, writeHealth *DatabaseHealth, status *Status) *DatabaseHealth {
	// Same connection - show that read uses the same as write
	if h.readDB == h.writeDB {
		return &DatabaseHealth{
			Connected: writeHealth.Connected,
			Driver:    writeHealth.Driver,
			Latency:   writeHealth.Latency,
			Host:      writeHealth.Host + " (shared with write)",
			Error:     writeHealth.Error,
		}
	}

	// No read database configured
	if h.readDB == nil {
		return nil
	}

	// Separate read database - check its health
	readHealth := h.checkDatabaseHealth(ctx, h.readDB, "read")
	if !readHealth.Connected && status.Status == StatusHealthy {
		// Write healthy but read unhealthy = degraded
		status.Status = StatusDegraded
	}

	return readHealth
}

// checkDatabaseHealth checks a specific database and returns its health status.
func (h *GORMHealthChecker) checkDatabaseHealth(ctx context.Context, db *gorm.DB, name string) *DatabaseHealth {
	dbHealth := &DatabaseHealth{}

	if db == nil {
		dbHealth.Connected = false
		dbHealth.Error = fmt.Sprintf("%s database not configured", name)

		return dbHealth
	}

	sqlDB, err := db.DB()
	if err != nil {
		dbHealth.Connected = false
		dbHealth.Error = fmt.Sprintf("failed to get underlying %s db: %v", name, err)

		return dbHealth
	}

	// Get driver name
	dbHealth.Driver = db.Name()

	// Get host from config if available
	if h.cfg != nil {
		switch name {
		case "write":
			dbHealth.Host = h.cfg.WriteDatabase.Host
		case "read":
			dbHealth.Host = h.cfg.ReadDatabase.Host
		}
	}

	// Time the ping
	start := time.Now()

	if err := sqlDB.PingContext(ctx); err != nil {
		dbHealth.Connected = false
		dbHealth.Error = fmt.Sprintf("ping failed: %v", err)
		dbHealth.Latency = time.Since(start)

		return dbHealth
	}

	dbHealth.Connected = true
	dbHealth.Latency = time.Since(start)

	return dbHealth
}

// pingDatabase pings a specific database connection.
func (h *GORMHealthChecker) pingDatabase(ctx context.Context, db *gorm.DB, name string) error {
	if db == nil {
		return fmt.Errorf("%s: %w", name, ErrDatabaseNotConfigured)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get underlying %s db: %w", name, err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping %s database: %w", name, err)
	}

	return nil
}
