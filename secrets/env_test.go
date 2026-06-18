package secrets_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/secrets"
)

func TestEnvProvider_GetSecret(t *testing.T) {
	ctx := context.Background()

	t.Run("returns secret from mapped env var", func(t *testing.T) {
		t.Setenv("DB_WRITE_PASSWORD", "test-write-password")

		provider := secrets.NewEnvProvider()
		value, err := provider.GetSecret(ctx, secrets.KeyDBWritePassword)

		require.NoError(t, err)
		assert.Equal(t, "test-write-password", value)
	})

	t.Run("returns secret from read password env var", func(t *testing.T) {
		t.Setenv("DB_READ_PASSWORD", "test-read-password")

		provider := secrets.NewEnvProvider()
		value, err := provider.GetSecret(ctx, secrets.KeyDBReadPassword)

		require.NoError(t, err)
		assert.Equal(t, "test-read-password", value)
	})

	t.Run("returns ErrSecretNotFound when env var not set", func(t *testing.T) {
		provider := secrets.NewEnvProvider()
		_, err := provider.GetSecret(ctx, secrets.KeyDBWritePassword)

		assert.ErrorIs(t, err, secrets.ErrSecretNotFound)
	})

	t.Run("uses key directly as env var name for unmapped keys", func(t *testing.T) {
		t.Setenv("MY_CUSTOM_SECRET", "custom-value")

		provider := secrets.NewEnvProvider()
		value, err := provider.GetSecret(ctx, "MY_CUSTOM_SECRET")

		require.NoError(t, err)
		assert.Equal(t, "custom-value", value)
	})
}

func TestEnvProvider_GetAllSecrets(t *testing.T) {
	ctx := context.Background()

	t.Run("returns all mapped secrets", func(t *testing.T) {
		t.Setenv("DB_WRITE_PASSWORD", "write-pass")
		t.Setenv("DB_READ_PASSWORD", "read-pass")

		provider := secrets.NewEnvProvider()
		all, err := provider.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Equal(t, "write-pass", all[secrets.KeyDBWritePassword])
		assert.Equal(t, "read-pass", all[secrets.KeyDBReadPassword])
	})

	t.Run("only includes secrets with non-empty values", func(t *testing.T) {
		t.Setenv("DB_WRITE_PASSWORD", "write-pass")
		// Don't set read password

		provider := secrets.NewEnvProvider()
		all, err := provider.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Equal(t, "write-pass", all[secrets.KeyDBWritePassword])
		_, hasReadPass := all[secrets.KeyDBReadPassword]
		assert.False(t, hasReadPass, "should not include unset secrets")
	})

	t.Run("returns empty map when no secrets set", func(t *testing.T) {
		provider := secrets.NewEnvProvider()
		all, err := provider.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Empty(t, all)
	})
}

func TestEnvProviderWithMapping(t *testing.T) {
	ctx := context.Background()

	t.Run("uses custom mapping", func(t *testing.T) {
		t.Setenv("CUSTOM_DB_PASS", "custom-password")

		mapping := map[string]string{ //nolint:gosec // test data, not real credentials
			"db_password": "CUSTOM_DB_PASS",
		}
		provider := secrets.NewEnvProviderWithMapping(mapping)

		value, err := provider.GetSecret(ctx, "db_password")

		require.NoError(t, err)
		assert.Equal(t, "custom-password", value)
	})

	t.Run("GetAllSecrets uses custom mapping", func(t *testing.T) {
		t.Setenv("CUSTOM_PASS_1", "pass1")
		t.Setenv("CUSTOM_PASS_2", "pass2")

		mapping := map[string]string{
			"secret1": "CUSTOM_PASS_1",
			"secret2": "CUSTOM_PASS_2",
		}
		provider := secrets.NewEnvProviderWithMapping(mapping)

		all, err := provider.GetAllSecrets(ctx)

		require.NoError(t, err)
		assert.Len(t, all, 2)
		assert.Equal(t, "pass1", all["secret1"])
		assert.Equal(t, "pass2", all["secret2"])
	})
}

func TestEnvProvider_ImplementsInterface(_ *testing.T) {
	// Compile-time check that EnvProvider implements Provider
	var _ secrets.Provider = (*secrets.EnvProvider)(nil)
}
