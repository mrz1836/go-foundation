package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheckerInterface ensures mock implementations satisfy the interface.
func TestHealthCheckerInterface(_ *testing.T) {
	// Verify MockHealthChecker implements Checker
	var _ Checker = (*MockHealthChecker)(nil)
}

// TestHealthCheckerContract verifies the interface contract behavior.
func TestHealthCheckerContract(t *testing.T) {
	t.Parallel()

	t.Run("Check returns nil when healthy", func(t *testing.T) {
		mock := NewHealthyMock()

		assert.NoError(t, mock.Check(context.Background()))
	})

	t.Run("Check returns error when unhealthy", func(t *testing.T) {
		mock := NewUnhealthyMock("connection failed")

		err := mock.Check(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrUnhealthy)
	})

	t.Run("CheckWithDetails returns healthy status", func(t *testing.T) {
		mock := NewHealthyMock()

		status, err := mock.CheckWithDetails(context.Background())
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, StatusHealthy, status.Status)
	})

	t.Run("CheckWithDetails returns unhealthy status", func(t *testing.T) {
		mock := NewUnhealthyMock("db timeout")

		status, err := mock.CheckWithDetails(context.Background())
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, StatusUnhealthy, status.Status)

		require.NotNil(t, status.WriteDatabase)
		assert.Equal(t, "db timeout", status.WriteDatabase.Error)
	})
}

// errCustom is a sentinel error for testing.
var errCustom = errors.New("custom error")

// TestMockHealthChecker_CustomBehavior tests configurable mock behavior.
func TestMockHealthChecker_CustomBehavior(t *testing.T) {
	t.Parallel()

	t.Run("custom Check function", func(t *testing.T) {
		mock := &MockHealthChecker{
			CheckFunc: func(_ context.Context) error {
				return errCustom
			},
		}

		err := mock.Check(context.Background())
		require.ErrorIs(t, err, errCustom)
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
		require.NoError(t, err)
		assert.Same(t, customStatus, status)
	})

	t.Run("call count tracking", func(t *testing.T) {
		mock := NewHealthyMock()

		assert.Equal(t, 0, mock.CallCount(), "initial call count should be 0")

		_ = mock.Check(context.Background())
		_ = mock.Check(context.Background())
		_, _ = mock.CheckWithDetails(context.Background())

		assert.Equal(t, 3, mock.CallCount())

		mock.Reset()

		assert.Equal(t, 0, mock.CallCount(), "call count should be 0 after reset")
	})
}

// TestStatusConstants verifies status constants are correct.
func TestStatusConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "healthy", StatusHealthy)
	assert.Equal(t, "degraded", StatusDegraded)
	assert.Equal(t, "unhealthy", StatusUnhealthy)
}
