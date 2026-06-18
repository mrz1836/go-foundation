package config

import (
	"context"
	"fmt"
)

// SecretsProvider is the interface for retrieving secrets. It mirrors the
// provider interface exposed by the secrets package and is declared here to
// avoid an import cycle.
type SecretsProvider interface {
	GetSecret(ctx context.Context, key string) (string, error)
	GetAllSecrets(ctx context.Context) (map[string]string, error)
}

// Secret key constants - must match the keys published by secret providers.
const (
	secretKeyDBWritePassword = "db_write_password"
	secretKeyDBReadPassword  = "db_read_password"
)

// ApplySecrets applies secrets from a provider to the configuration.
// This is called after loading config to populate sensitive fields from a
// secrets manager.
//
// The function retrieves all secrets from the provider and maps them to config fields:
//   - db_write_password -> WriteDatabase.Password
//   - db_read_password  -> ReadDatabase.Password
//
// If a secret is not found in the provider, the existing config value is preserved.
func ApplySecrets(ctx context.Context, cfg *Config, provider SecretsProvider) error {
	if provider == nil {
		return nil
	}

	secrets, err := provider.GetAllSecrets(ctx)
	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}

	// Apply database passwords if present in secrets
	if password, ok := secrets[secretKeyDBWritePassword]; ok && password != "" {
		cfg.WriteDatabase.Password = password
	}

	if password, ok := secrets[secretKeyDBReadPassword]; ok && password != "" {
		cfg.ReadDatabase.Password = password
	}

	return nil
}
