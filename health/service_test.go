package health

import (
	"context"
	"errors"
	"testing"
)

// TestHealthCheckerInterface ensures mock implementations satisfy the interface.
func TestHealthCheckerInterface(_ *testing.T) {
	// Verify MockHealthChecker implements Checker
	var _ Checker = (*MockHealthChecker)(nil)
}

// TestHealthCheckerContract verifies the interface contract behavior.
//
//nolint:gocognit,gocyclo // Test function with multiple sub-tests
func TestHealthCheckerContract(t *testing.T) {
	t.Parallel()

	t.Run("Check returns nil when healthy", func(t *testing.T) {
		mock := NewHealthyMock()

		err := mock.Check(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("Check returns error when unhealthy", func(t *testing.T) {
		mock := NewUnhealthyMock("connection failed")

		err := mock.Check(context.Background())
		if err == nil {
			t.Error("expected error, got nil")
		}

		if !errors.Is(err, ErrUnhealthy) {
			t.Errorf("expected ErrUnhealthy, got: %v", err)
		}
	})

	t.Run("CheckWithDetails returns healthy status", func(t *testing.T) {
		mock := NewHealthyMock()

		status, err := mock.CheckWithDetails(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if status == nil {
			t.Fatal("expected status, got nil")
		}

		if status.Status != StatusHealthy {
			t.Errorf("expected status %s, got %s", StatusHealthy, status.Status)
		}
	})

	t.Run("CheckWithDetails returns unhealthy status", func(t *testing.T) {
		mock := NewUnhealthyMock("db timeout")

		status, err := mock.CheckWithDetails(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if status == nil {
			t.Fatal("expected status, got nil")
		}

		if status.Status != StatusUnhealthy {
			t.Errorf("expected status %s, got %s", StatusUnhealthy, status.Status)
		}

		if status.WriteDatabase == nil {
			t.Fatal("expected write database health info")
		}

		if status.WriteDatabase.Error != "db timeout" {
			t.Errorf("expected error 'db timeout', got %s", status.WriteDatabase.Error)
		}
	})
}

// errCustom is a sentinel error for testing.
var errCustom = errors.New("custom error")

// TestMockHealthChecker_CustomBehavior tests configurable mock behavior.
//
//nolint:gocognit // Test function with multiple sub-tests
func TestMockHealthChecker_CustomBehavior(t *testing.T) {
	t.Parallel()

	t.Run("custom Check function", func(t *testing.T) {
		mock := &MockHealthChecker{
			CheckFunc: func(_ context.Context) error {
				return errCustom
			},
		}

		err := mock.Check(context.Background())
		if !errors.Is(err, errCustom) {
			t.Errorf("expected custom error, got: %v", err)
		}
	})

	t.Run("custom CheckWithDetails function", func(t *testing.T) {
		customStatus := &Status{
			Status:      StatusDegraded,
			Version:     "custom",
			Environment: "test",
		}
		mock := &MockHealthChecker{
			CheckWithDetailsFunc: func(_ context.Context) (*Status, error) {
				return customStatus, nil
			},
		}

		status, err := mock.CheckWithDetails(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if status != customStatus {
			t.Error("expected custom status")
		}
	})

	t.Run("call count tracking", func(t *testing.T) {
		mock := NewHealthyMock()

		if mock.CallCount() != 0 {
			t.Error("initial call count should be 0")
		}

		_ = mock.Check(context.Background())
		_ = mock.Check(context.Background())
		_, _ = mock.CheckWithDetails(context.Background())

		if mock.CallCount() != 3 {
			t.Errorf("expected 3 calls, got %d", mock.CallCount())
		}

		mock.Reset()

		if mock.CallCount() != 0 {
			t.Error("call count should be 0 after reset")
		}
	})
}

// TestStatusConstants verifies status constants are correct.
func TestStatusConstants(t *testing.T) {
	t.Parallel()

	if StatusHealthy != "healthy" {
		t.Errorf("StatusHealthy = %s, want healthy", StatusHealthy)
	}

	if StatusDegraded != "degraded" {
		t.Errorf("StatusDegraded = %s, want degraded", StatusDegraded)
	}

	if StatusUnhealthy != "unhealthy" {
		t.Errorf("StatusUnhealthy = %s, want unhealthy", StatusUnhealthy)
	}
}
