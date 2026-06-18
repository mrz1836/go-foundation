package secrets

import (
	"context"
	"sync/atomic"
)

// MockProvider is a configurable mock for testing secrets retrieval.
// It allows customizing the behavior of GetSecret and GetAllSecrets methods.
type MockProvider struct {
	// GetSecretFunc is called when GetSecret is invoked.
	// If nil, returns values from the Secrets map.
	GetSecretFunc func(ctx context.Context, key string) (string, error)

	// GetAllSecretsFunc is called when GetAllSecrets is invoked.
	// If nil, returns a copy of the Secrets map.
	GetAllSecretsFunc func(ctx context.Context) (map[string]string, error)

	// Secrets holds the mock secret values for default behavior.
	Secrets map[string]string

	// Call counts for verification in tests
	getSecretCalls     atomic.Int64
	getAllSecretsCalls atomic.Int64
}

// NewMockProvider creates a new mock provider with empty secrets.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		Secrets: make(map[string]string),
	}
}

// NewMockProviderWithSecrets creates a mock provider with predefined secrets.
func NewMockProviderWithSecrets(secrets map[string]string) *MockProvider {
	return &MockProvider{
		Secrets: secrets,
	}
}

// NewTestSecretsProvider creates a mock provider with typical test values.
// Use this in unit tests that need database credentials.
func NewTestSecretsProvider() *MockProvider {
	return NewMockProviderWithSecrets(map[string]string{
		KeyDBWritePassword: "test-write-password",
		KeyDBReadPassword:  "test-read-password",
	})
}

// GetSecret retrieves a secret value from the mock.
func (m *MockProvider) GetSecret(ctx context.Context, key string) (string, error) {
	m.getSecretCalls.Add(1)

	if m.GetSecretFunc != nil {
		return m.GetSecretFunc(ctx, key)
	}

	value, ok := m.Secrets[key]
	if !ok {
		return "", ErrSecretNotFound
	}

	return value, nil
}

// GetAllSecrets retrieves all secrets from the mock.
func (m *MockProvider) GetAllSecrets(ctx context.Context) (map[string]string, error) {
	m.getAllSecretsCalls.Add(1)

	if m.GetAllSecretsFunc != nil {
		return m.GetAllSecretsFunc(ctx)
	}

	// Return a copy to prevent external modification
	result := make(map[string]string, len(m.Secrets))
	for k, v := range m.Secrets {
		result[k] = v
	}

	return result, nil
}

// GetSecretCallCount returns the number of times GetSecret was called.
func (m *MockProvider) GetSecretCallCount() int64 {
	return m.getSecretCalls.Load()
}

// GetAllSecretsCallCount returns the number of times GetAllSecrets was called.
func (m *MockProvider) GetAllSecretsCallCount() int64 {
	return m.getAllSecretsCalls.Load()
}

// ResetCallCounts resets all call counters to zero.
func (m *MockProvider) ResetCallCounts() {
	m.getSecretCalls.Store(0)
	m.getAllSecretsCalls.Store(0)
}

// SetSecret sets a single secret value in the mock.
func (m *MockProvider) SetSecret(key, value string) {
	if m.Secrets == nil {
		m.Secrets = make(map[string]string)
	}

	m.Secrets[key] = value
}
