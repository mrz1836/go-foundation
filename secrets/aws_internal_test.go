package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testInternalSecretARN is a sample (non-secret) ARN used to construct providers.
const testInternalSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:test" //nolint:gosec // sample ARN, not a credential

// errInternalConfigBoom is a package-scope sentinel (keeps err113 happy) used to
// force the AWS config-load failure branch.
var errInternalConfigBoom = errors.New("config boom")

// TestNewAWSProvider_LoadConfigError verifies that NewAWSProvider wraps a
// failure from AWS config loading with ErrAWSSecretsManager. LoadDefaultConfig
// succeeds offline, so the injectable loadAWSConfig var is overridden here to
// force the error and cover the constructor's error branch.
func TestNewAWSProvider_LoadConfigError(t *testing.T) {
	// Not parallel: this test swaps the package-level loadAWSConfig var.
	loadErr := errInternalConfigBoom

	original := loadAWSConfig
	loadAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, loadErr
	}
	t.Cleanup(func() { loadAWSConfig = original })

	provider, err := NewAWSProvider(context.Background(), testInternalSecretARN)

	assert.Nil(t, provider, "expected nil provider when config loading fails")
	require.ErrorIs(t, err, ErrAWSSecretsManager)
	require.ErrorIs(t, err, loadErr)
}
