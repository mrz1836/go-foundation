package health

import (
	"context"
	"sync/atomic"
	"time"
)

// MockHealthChecker is a mock implementation of Checker for testing.
type MockHealthChecker struct {
	// CheckFunc is called by Check if set.
	CheckFunc func(ctx context.Context) error

	// CheckWithDetailsFunc is called by CheckWithDetails if set.
	CheckWithDetailsFunc func(ctx context.Context) (*Status, error)

	// callCount tracks the number of calls to any method.
	callCount atomic.Int64
}

// NewHealthyMock creates a mock that always returns healthy status.
func NewHealthyMock() *MockHealthChecker {
	return &MockHealthChecker{}
}

// NewUnhealthyMock creates a mock that returns unhealthy status with the given error message.
func NewUnhealthyMock(errMsg string) *MockHealthChecker {
	err := ErrUnhealthy

	return &MockHealthChecker{
		CheckFunc: func(_ context.Context) error {
			return err
		},
		CheckWithDetailsFunc: func(_ context.Context) (*Status, error) {
			return &Status{
				Status:    StatusUnhealthy,
				Timestamp: time.Now(),
				WriteDatabase: &DatabaseHealth{
					Connected: false,
					Driver:    "mock",
					Latency:   0,
					Error:     errMsg,
				},
				ReadDatabase: &DatabaseHealth{
					Connected: false,
					Driver:    "mock",
					Latency:   0,
					Error:     errMsg,
				},
			}, nil
		},
	}
}

// Check calls CheckFunc if set, otherwise returns nil.
func (m *MockHealthChecker) Check(ctx context.Context) error {
	m.callCount.Add(1)

	if m.CheckFunc != nil {
		return m.CheckFunc(ctx)
	}

	return nil
}

// CheckWithDetails calls CheckWithDetailsFunc if set, otherwise returns a healthy status.
func (m *MockHealthChecker) CheckWithDetails(ctx context.Context) (*Status, error) {
	m.callCount.Add(1)

	if m.CheckWithDetailsFunc != nil {
		return m.CheckWithDetailsFunc(ctx)
	}

	return &Status{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		WriteDatabase: &DatabaseHealth{
			Connected: true,
			Driver:    "mock",
			Latency:   time.Millisecond,
		},
		ReadDatabase: &DatabaseHealth{
			Connected: true,
			Driver:    "mock",
			Latency:   time.Millisecond,
		},
	}, nil
}

// CallCount returns the number of times any method was called.
func (m *MockHealthChecker) CallCount() int {
	return int(m.callCount.Load())
}

// Reset resets the call count to zero.
func (m *MockHealthChecker) Reset() {
	m.callCount.Store(0)
}
