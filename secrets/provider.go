// Package secrets provides a unified interface for retrieving secrets from various sources.
//
// This package follows the standard service pattern used throughout the services layer:
//
//  1. Interface Definition (provider.go): Defines the Provider interface for retrieving secrets.
//
//  2. Environment Implementation (env.go): Reads secrets from environment variables.
//     Use this for local development and testing.
//
//  3. AWS Implementation (aws.go): Reads secrets from AWS Secrets Manager.
//     Use this for Lambda/production deployments.
//
//  4. Mock Implementation (mock.go): Provides a configurable mock for testing.
//
// # Usage
//
// In local/test environments:
//
//	provider := secrets.NewEnvProvider()
//	password, err := provider.GetSecret(ctx, "db_write_password")
//
// In Lambda/production:
//
//	provider, err := secrets.NewAWSProvider(ctx, secretARN)
//	allSecrets, err := provider.GetAllSecrets(ctx)
package secrets

import (
	"context"
	"errors"
)

// Sentinel errors for secrets operations.
var (
	// ErrSecretNotFound is returned when a requested secret key does not exist.
	ErrSecretNotFound = errors.New("secret not found")

	// ErrProviderNotInitialized is returned when the provider is used before initialization.
	ErrProviderNotInitialized = errors.New("secrets provider not initialized")

	// ErrInvalidSecretFormat is returned when the secret data cannot be parsed.
	ErrInvalidSecretFormat = errors.New("invalid secret format")

	// ErrAWSSecretsManager is returned when AWS Secrets Manager operations fail.
	ErrAWSSecretsManager = errors.New("AWS Secrets Manager error")
)

// Secret key constants - these are the keys used in the JSON secret stored in Secrets Manager.
// They map to the environment variable suffixes (without the project-specific prefix).
const (
	// KeyDBWritePassword is the key for the write database password.
	KeyDBWritePassword = "db_write_password"

	// KeyDBReadPassword is the key for the read database password.
	KeyDBReadPassword = "db_read_password"
)

// Provider defines the interface for retrieving secrets.
// Implementations can read from environment variables, AWS Secrets Manager, or other sources.
type Provider interface {
	// GetSecret retrieves a single secret value by key.
	// Returns ErrSecretNotFound if the key does not exist.
	GetSecret(ctx context.Context, key string) (string, error)

	// GetAllSecrets retrieves all secrets as a key-value map.
	// This is more efficient when multiple secrets are needed.
	GetAllSecrets(ctx context.Context) (map[string]string, error)
}
