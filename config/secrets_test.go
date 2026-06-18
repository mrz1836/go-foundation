package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/config"
)

var errConnectionFailed = errors.New("connection failed")

type mockSecretsProvider struct {
	secrets map[string]string
	err     error
}

func (m *mockSecretsProvider) GetSecret(_ context.Context, key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}

	return m.secrets[key], nil
}

func (m *mockSecretsProvider) GetAllSecrets(_ context.Context) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.secrets, nil
}

func TestApplySecrets_NilProvider(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.WriteDatabase.Password = "original_write"
	cfg.ReadDatabase.Password = "original_read"

	err := config.ApplySecrets(context.Background(), &cfg, nil)

	require.NoError(t, err)
	assert.Equal(t, "original_write", cfg.WriteDatabase.Password)
	assert.Equal(t, "original_read", cfg.ReadDatabase.Password)
}

func TestApplySecrets_ProviderError(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	provider := &mockSecretsProvider{err: errConnectionFailed}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.Error(t, err)
	require.ErrorIs(t, err, errConnectionFailed)
	assert.Contains(t, err.Error(), "failed to get secrets")
}

func TestApplySecrets_BothPasswordsApplied(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	provider := &mockSecretsProvider{
		secrets: map[string]string{
			"db_write_password": "new_write_pass",
			"db_read_password":  "new_read_pass",
		},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "new_write_pass", cfg.WriteDatabase.Password)
	assert.Equal(t, "new_read_pass", cfg.ReadDatabase.Password)
}

func TestApplySecrets_OnlyWritePasswordApplied(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.ReadDatabase.Password = "original_read"
	provider := &mockSecretsProvider{
		secrets: map[string]string{
			"db_write_password": "new_write_pass",
		},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "new_write_pass", cfg.WriteDatabase.Password)
	assert.Equal(t, "original_read", cfg.ReadDatabase.Password)
}

func TestApplySecrets_OnlyReadPasswordApplied(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.WriteDatabase.Password = "original_write"
	provider := &mockSecretsProvider{
		secrets: map[string]string{
			"db_read_password": "new_read_pass",
		},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "original_write", cfg.WriteDatabase.Password)
	assert.Equal(t, "new_read_pass", cfg.ReadDatabase.Password)
}

func TestApplySecrets_EmptyValueNotApplied(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.WriteDatabase.Password = "original_write"
	cfg.ReadDatabase.Password = "original_read"
	provider := &mockSecretsProvider{
		secrets: map[string]string{
			"db_write_password": "",
			"db_read_password":  "",
		},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "original_write", cfg.WriteDatabase.Password)
	assert.Equal(t, "original_read", cfg.ReadDatabase.Password)
}

func TestApplySecrets_NoRelevantKeys(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.WriteDatabase.Password = "original_write"
	cfg.ReadDatabase.Password = "original_read"
	provider := &mockSecretsProvider{
		secrets: map[string]string{
			"some_other_key": "some_value",
		},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "original_write", cfg.WriteDatabase.Password)
	assert.Equal(t, "original_read", cfg.ReadDatabase.Password)
}

func TestApplySecrets_EmptySecrets(t *testing.T) {
	t.Parallel()

	cfg := newValidConfig()
	cfg.WriteDatabase.Password = "original_write"
	cfg.ReadDatabase.Password = "original_read"
	provider := &mockSecretsProvider{
		secrets: map[string]string{},
	}

	err := config.ApplySecrets(context.Background(), &cfg, provider)

	require.NoError(t, err)
	assert.Equal(t, "original_write", cfg.WriteDatabase.Password)
	assert.Equal(t, "original_read", cfg.ReadDatabase.Password)
}
