// Package health provides health check services for database connections.
//
// # Service Interface Pattern
//
// This package demonstrates the standard service pattern used throughout the
// services layer:
//
//  1. Interface Definition (service.go): Define a small, focused interface
//     with 1-5 methods that describe the service behavior.
//
//  2. Production Implementation (gorm.go): Implement the interface using
//     real dependencies (e.g., GORM for database operations).
//
//  3. Mock Implementation (mock.go): Provide a configurable mock for testing
//     with function fields that can be customized per test.
//
// # Adding a New Service
//
// To add a new service (e.g., UserService), follow this same pattern:
//
//	// services/user/service.go
//	type UserService interface {
//	    GetByID(ctx context.Context, id string) (*User, error)
//	    Create(ctx context.Context, data CreateUserInput) (*User, error)
//	}
//
//	// services/user/gorm.go
//	type GORMUserService struct { db *gorm.DB }
//	func (s *GORMUserService) GetByID(ctx context.Context, id string) (*User, error) { ... }
//
//	// services/user/mock.go
//	type MockUserService struct {
//	    GetByIDFunc func(ctx context.Context, id string) (*User, error)
//	}
package health

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for health check operations.
var (
	// ErrUnhealthy is the sentinel error returned when a health check fails.
	ErrUnhealthy = errors.New("health check failed")

	// ErrDatabaseNotConfigured is returned when a database connection is nil.
	ErrDatabaseNotConfigured = errors.New("database not configured")
)

// Status constants for health check results.
const (
	// StatusHealthy indicates all services are operational.
	StatusHealthy = "healthy"

	// StatusDegraded indicates some services have issues but are functional.
	StatusDegraded = "degraded"

	// StatusUnhealthy indicates critical services are not operational.
	StatusUnhealthy = "unhealthy"
)

// Checker defines the interface for health check operations.
type Checker interface {
	// Check performs a basic health check and returns an error if unhealthy.
	Check(ctx context.Context) error

	// CheckWithDetails performs a detailed health check with diagnostic information.
	CheckWithDetails(ctx context.Context) (*Status, error)
}

// Status contains detailed health information for diagnostics.
type Status struct {
	Status        string          `json:"status"`
	WriteDatabase *DatabaseHealth `json:"write_database,omitempty"`
	ReadDatabase  *DatabaseHealth `json:"read_database,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
	Version       string          `json:"version"`
	Environment   string          `json:"environment"`
}

// DatabaseHealth contains database-specific health information.
type DatabaseHealth struct {
	Connected bool          `json:"connected"`
	Driver    string        `json:"driver"`
	Latency   time.Duration `json:"latency_ms"`
	Host      string        `json:"host,omitempty"`
	Error     string        `json:"error,omitempty"`
}
