package secrets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/secrets"
)

var errUnknownKey = errors.New("unknown key")

func TestAWSProvider_GetSecret_WithoutClient(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create provider without client to test error handling
	provider := secrets.NewAWSProviderWithClient(nil, "arn:aws:secretsmanager:us-east-1:123456789:secret:test")

	_, err := provider.GetSecret(ctx, secrets.KeyDBWritePassword)

	require.ErrorIs(t, err, secrets.ErrProviderNotInitialized)
}

func TestAWSProvider_Refresh(t *testing.T) {
	t.Parallel()

	// Create a provider with nil client
	provider := secrets.NewAWSProviderWithClient(nil, "arn:aws:secretsmanager:us-east-1:123456789:secret:test")

	// First call should fail with not initialized
	_, err := provider.GetAllSecrets(context.Background())
	require.ErrorIs(t, err, secrets.ErrProviderNotInitialized)

	// Refresh clears the cache (shouldn't panic)
	provider.Refresh()

	// After refresh, calling again should still fail (same nil client)
	_, err = provider.GetAllSecrets(context.Background())
	require.ErrorIs(t, err, secrets.ErrProviderNotInitialized)
}

func TestAWSProvider_ImplementsInterface(_ *testing.T) {
	// Compile-time check that AWSProvider implements Provider
	var _ secrets.Provider = (*secrets.AWSProvider)(nil)
}

// MockProvider tests - verify the mock works correctly for use in other tests

func TestMockProvider_GetSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns secret from map", func(t *testing.T) {
		mock := secrets.NewMockProviderWithSecrets(map[string]string{
			"my_secret": "secret-value",
		})

		value, err := mock.GetSecret(ctx, "my_secret")

		require.NoError(t, err)
		assert.Equal(t, "secret-value", value)
	})

	t.Run("returns ErrSecretNotFound for missing key", func(t *testing.T) {
		mock := secrets.NewMockProvider()

		_, err := mock.GetSecret(ctx, "nonexistent")

		assert.ErrorIs(t, err, secrets.ErrSecretNotFound)
	})

	t.Run("uses custom GetSecretFunc when provided", func(t *testing.T) {
		mock := secrets.NewMockProvider()
		mock.GetSecretFunc = func(_ context.Context, key string) (string, error) {
			if key == "special" {
				return "special-value", nil
			}

			return "", errUnknownKey
		}

		value, err := mock.GetSecret(ctx, "special")
		require.NoError(t, err)
		assert.Equal(t, "special-value", value)

		_, err = mock.GetSecret(ctx, "other")
		assert.Error(t, err)
	})
}

func TestMockProvider_GetAllSecrets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns copy of secrets map", func(t *testing.T) {
		mock := secrets.NewMockProviderWithSecrets(map[string]string{
			"key1": "value1",
			"key2": "value2",
		})

		all, err := mock.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Len(t, all, 2)
		assert.Equal(t, "value1", all["key1"])
		assert.Equal(t, "value2", all["key2"])

		// Modifying the returned map shouldn't affect the mock
		all["key1"] = "modified"
		value, _ := mock.GetSecret(ctx, "key1")
		assert.Equal(t, "value1", value, "original should be unchanged")
	})

	t.Run("uses custom GetAllSecretsFunc when provided", func(t *testing.T) {
		mock := secrets.NewMockProvider()
		mock.GetAllSecretsFunc = func(_ context.Context) (map[string]string, error) {
			return map[string]string{"custom": "result"}, nil
		}

		all, err := mock.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Equal(t, "result", all["custom"])
	})
}

func TestMockProvider_CallCounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := secrets.NewMockProviderWithSecrets(map[string]string{
		"key": "value",
	})

	assert.Equal(t, int64(0), mock.GetSecretCallCount())
	assert.Equal(t, int64(0), mock.GetAllSecretsCallCount())

	_, _ = mock.GetSecret(ctx, "key")
	_, _ = mock.GetSecret(ctx, "key")

	assert.Equal(t, int64(2), mock.GetSecretCallCount())

	_, _ = mock.GetAllSecrets(ctx)

	assert.Equal(t, int64(1), mock.GetAllSecretsCallCount())

	mock.ResetCallCounts()

	assert.Equal(t, int64(0), mock.GetSecretCallCount())
	assert.Equal(t, int64(0), mock.GetAllSecretsCallCount())
}

func TestMockProvider_SetSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := secrets.NewMockProvider()

	// Initially empty
	_, err := mock.GetSecret(ctx, "new_key")
	require.ErrorIs(t, err, secrets.ErrSecretNotFound)

	// Set a secret
	mock.SetSecret("new_key", "new_value")

	// Now it should exist
	value, err := mock.GetSecret(ctx, "new_key")
	require.NoError(t, err)
	assert.Equal(t, "new_value", value)
}

func TestMockProvider_SetSecret_NilMap(t *testing.T) {
	t.Parallel()

	// A zero-value MockProvider has a nil Secrets map; SetSecret must lazily
	// initialize it rather than panic.
	mock := &secrets.MockProvider{}
	mock.SetSecret("lazy_key", "lazy_value")

	value, err := mock.GetSecret(context.Background(), "lazy_key")
	require.NoError(t, err)
	assert.Equal(t, "lazy_value", value)
}

func TestNewTestSecretsProvider(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	provider := secrets.NewTestSecretsProvider()

	// Should have standard test credentials
	writePass, err := provider.GetSecret(ctx, secrets.KeyDBWritePassword)
	require.NoError(t, err)
	assert.Equal(t, "test-write-password", writePass)

	readPass, err := provider.GetSecret(ctx, secrets.KeyDBReadPassword)
	require.NoError(t, err)
	assert.Equal(t, "test-read-password", readPass)
}

func TestMockProvider_ImplementsInterface(_ *testing.T) {
	// Compile-time check that MockProvider implements Provider
	var _ secrets.Provider = (*secrets.MockProvider)(nil)
}

// Test sentinel errors

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	// Ensure sentinel errors are unique
	assert.NotEqual(t, secrets.ErrSecretNotFound, secrets.ErrProviderNotInitialized)
	assert.NotEqual(t, secrets.ErrSecretNotFound, secrets.ErrInvalidSecretFormat)
	assert.NotEqual(t, secrets.ErrProviderNotInitialized, secrets.ErrAWSSecretsManager)

	// Ensure error messages are meaningful
	assert.Contains(t, secrets.ErrSecretNotFound.Error(), "not found")
	assert.Contains(t, secrets.ErrProviderNotInitialized.Error(), "not initialized")
	assert.Contains(t, secrets.ErrInvalidSecretFormat.Error(), "invalid")
	assert.Contains(t, secrets.ErrAWSSecretsManager.Error(), "AWS")
}

// Test secret key constants

func TestSecretKeyConstants(t *testing.T) {
	t.Parallel()

	// Verify key constants are defined and non-empty
	assert.NotEmpty(t, secrets.KeyDBWritePassword)
	assert.NotEmpty(t, secrets.KeyDBReadPassword)

	// Verify they're different from each other
	assert.NotEqual(t, secrets.KeyDBWritePassword, secrets.KeyDBReadPassword)
}
