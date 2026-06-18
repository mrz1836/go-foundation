package secrets

import (
	"context"
	"os"
)

// defaultEnvKeyMapping returns the default mapping of secret keys to environment variable names.
// This allows the EnvProvider to read secrets from the same env vars
// that are used when secrets are injected directly (local/test mode).
func defaultEnvKeyMapping() map[string]string {
	return map[string]string{
		KeyDBWritePassword: "DB_WRITE_PASSWORD",
		KeyDBReadPassword:  "DB_READ_PASSWORD",
	}
}

// EnvProvider reads secrets from environment variables.
// Use this for local development and testing where secrets are set directly
// in environment variables rather than fetched from AWS Secrets Manager.
type EnvProvider struct {
	// keyMapping allows overriding the default env var names for testing
	keyMapping map[string]string
}

// NewEnvProvider creates a new environment-based secrets provider.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{
		keyMapping: defaultEnvKeyMapping(),
	}
}

// NewEnvProviderWithMapping creates an EnvProvider with custom key-to-envvar mapping.
// This is useful for testing with custom environment variable names.
func NewEnvProviderWithMapping(mapping map[string]string) *EnvProvider {
	return &EnvProvider{
		keyMapping: mapping,
	}
}

// GetSecret retrieves a secret value from environment variables.
// The key is mapped to an environment variable name using the standard mapping.
func (p *EnvProvider) GetSecret(_ context.Context, key string) (string, error) {
	envVar, ok := p.keyMapping[key]
	if !ok {
		// If no mapping exists, try using the key directly as an env var name
		envVar = key
	}

	value := os.Getenv(envVar)
	if value == "" {
		return "", ErrSecretNotFound
	}

	return value, nil
}

// GetAllSecrets retrieves all mapped secrets from environment variables.
// Only returns secrets that have non-empty values.
func (p *EnvProvider) GetAllSecrets(_ context.Context) (map[string]string, error) {
	secrets := make(map[string]string)

	for key, envVar := range p.keyMapping {
		if value := os.Getenv(envVar); value != "" {
			secrets[key] = value
		}
	}

	return secrets, nil
}
