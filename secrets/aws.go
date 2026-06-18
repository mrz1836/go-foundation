package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// AWSProvider reads secrets from AWS Secrets Manager.
// It fetches a single JSON secret and caches the parsed values in memory.
// Use this for Lambda/production deployments.
type AWSProvider struct {
	client    *secretsmanager.Client
	secretARN string

	// Cached secrets (populated on first access)
	cache     map[string]string
	cacheOnce sync.Once
	cacheErr  error
}

// NewAWSProvider creates a new AWS Secrets Manager provider.
// The secretARN should point to a JSON secret containing key-value pairs.
//
// Example secret format in Secrets Manager:
//
//	{
//	  "db_write_password": "secret123",
//	  "db_read_password": "secret456",
//	  "api_key": "apikey789"
//	}
func NewAWSProvider(ctx context.Context, secretARN string) (*AWSProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", ErrAWSSecretsManager, err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	return &AWSProvider{
		client:    client,
		secretARN: secretARN,
	}, nil
}

// NewAWSProviderWithClient creates an AWSProvider with a custom Secrets Manager client.
// This is useful for testing with mock clients.
func NewAWSProviderWithClient(client *secretsmanager.Client, secretARN string) *AWSProvider {
	return &AWSProvider{
		client:    client,
		secretARN: secretARN,
	}
}

// GetSecret retrieves a single secret value by key.
// On first call, fetches and caches all secrets from Secrets Manager.
func (p *AWSProvider) GetSecret(ctx context.Context, key string) (string, error) {
	secrets, err := p.GetAllSecrets(ctx)
	if err != nil {
		return "", err
	}

	value, ok := secrets[key]
	if !ok {
		return "", fmt.Errorf("%w: key %q not found in secret", ErrSecretNotFound, key)
	}

	return value, nil
}

// GetAllSecrets retrieves all secrets from AWS Secrets Manager.
// Results are cached in memory after the first call.
func (p *AWSProvider) GetAllSecrets(ctx context.Context) (map[string]string, error) {
	p.cacheOnce.Do(func() {
		p.cache, p.cacheErr = p.fetchSecrets(ctx)
	})

	if p.cacheErr != nil {
		return nil, p.cacheErr
	}

	// Return a copy to prevent external modification of cache
	result := make(map[string]string, len(p.cache))
	for k, v := range p.cache {
		result[k] = v
	}

	return result, nil
}

// Refresh clears the cache and forces a refresh on the next GetSecret call.
// This can be useful for long-running processes that need to pick up rotated secrets.
func (p *AWSProvider) Refresh() {
	p.cacheOnce = sync.Once{}
	p.cache = nil
	p.cacheErr = nil
}

// fetchSecrets retrieves and parses the JSON secret from Secrets Manager.
func (p *AWSProvider) fetchSecrets(ctx context.Context) (map[string]string, error) {
	if p.client == nil {
		return nil, ErrProviderNotInitialized
	}

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &p.secretARN,
	}

	result, err := p.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get secret value: %w", ErrAWSSecretsManager, err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("%w: secret has no string value (binary secrets not supported)", ErrInvalidSecretFormat)
	}

	var secrets map[string]string
	if err := json.Unmarshal([]byte(*result.SecretString), &secrets); err != nil {
		return nil, fmt.Errorf("%w: failed to parse JSON secret: %w", ErrInvalidSecretFormat, err)
	}

	return secrets, nil
}
