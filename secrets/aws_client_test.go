package secrets_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/secrets"
)

// testSecretARN is a sample (non-secret) ARN used to construct providers.
const testSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:test" //nolint:gosec // sample ARN, not a credential

// stubHTTPClient is an aws.HTTPClient that returns a canned response, letting
// the real Secrets Manager client run its (de)serialization without a network
// call. It counts invocations so tests can assert the provider caches results.
type stubHTTPClient struct {
	status int
	body   string
	calls  atomic.Int64
}

func (s *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)

	return &http.Response{
		StatusCode: s.status,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// newStubbedAWSProvider builds an AWSProvider backed by a real Secrets Manager
// client whose transport returns the supplied canned response. Anonymous
// credentials skip request signing so the test never touches AWS.
func newStubbedAWSProvider(t *testing.T, status int, body string) (*secrets.AWSProvider, *stubHTTPClient) {
	t.Helper()

	stub := &stubHTTPClient{status: status, body: body}
	client := secretsmanager.New(secretsmanager.Options{
		Region:      "us-east-1",
		Credentials: aws.AnonymousCredentials{},
		HTTPClient:  stub,
	})

	return secrets.NewAWSProviderWithClient(client, testSecretARN), stub
}

func TestAWSProvider_GetAllSecrets_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	body := `{"SecretString":"{\"db_write_password\":\"pw123\",\"db_read_password\":\"pw456\"}"}`
	provider, stub := newStubbedAWSProvider(t, http.StatusOK, body)

	all, err := provider.GetAllSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pw123", all[secrets.KeyDBWritePassword])
	assert.Equal(t, "pw456", all[secrets.KeyDBReadPassword])

	// Mutating the returned map must not affect the cached copy.
	all[secrets.KeyDBWritePassword] = "tampered"
	again, err := provider.GetAllSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pw123", again[secrets.KeyDBWritePassword], "cache must return a fresh copy")

	// The secret is fetched once and cached for subsequent reads.
	assert.Equal(t, int64(1), stub.calls.Load(), "GetSecretValue must be called only once")
}

func TestAWSProvider_GetSecret_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	body := `{"SecretString":"{\"db_write_password\":\"pw123\"}"}`
	provider, _ := newStubbedAWSProvider(t, http.StatusOK, body)

	t.Run("returns the value for a known key", func(t *testing.T) {
		value, err := provider.GetSecret(ctx, secrets.KeyDBWritePassword)
		require.NoError(t, err)
		assert.Equal(t, "pw123", value)
	})

	t.Run("returns ErrSecretNotFound for an unknown key", func(t *testing.T) {
		_, err := provider.GetSecret(ctx, "missing_key")
		require.ErrorIs(t, err, secrets.ErrSecretNotFound)
	})
}

func TestAWSProvider_GetAllSecrets_NoSecretString(t *testing.T) {
	t.Parallel()

	// A 200 response without a SecretString (e.g. a binary secret) is rejected.
	provider, _ := newStubbedAWSProvider(t, http.StatusOK, `{"Name":"test"}`)

	_, err := provider.GetAllSecrets(context.Background())
	require.ErrorIs(t, err, secrets.ErrInvalidSecretFormat)
}

func TestAWSProvider_GetAllSecrets_InvalidJSON(t *testing.T) {
	t.Parallel()

	// SecretString that is not a JSON object cannot be parsed into the map.
	provider, _ := newStubbedAWSProvider(t, http.StatusOK, `{"SecretString":"not-json"}`)

	_, err := provider.GetAllSecrets(context.Background())
	require.ErrorIs(t, err, secrets.ErrInvalidSecretFormat)
}

func TestAWSProvider_GetAllSecrets_APIError(t *testing.T) {
	t.Parallel()

	// A non-2xx response surfaces as a wrapped Secrets Manager error.
	body := `{"__type":"ResourceNotFoundException","Message":"secret not found"}`
	provider, _ := newStubbedAWSProvider(t, http.StatusBadRequest, body)

	_, err := provider.GetAllSecrets(context.Background())
	require.ErrorIs(t, err, secrets.ErrAWSSecretsManager)
}

func TestAWSProvider_Refresh_RefetchesAfterCache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	body := `{"SecretString":"{\"db_write_password\":\"pw123\"}"}`
	provider, stub := newStubbedAWSProvider(t, http.StatusOK, body)

	_, err := provider.GetAllSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stub.calls.Load())

	// Refresh clears the cache, so the next read re-fetches from the backend.
	provider.Refresh()

	_, err = provider.GetAllSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stub.calls.Load(), "Refresh must force a re-fetch")
}

func TestNewAWSProvider_Constructs(t *testing.T) {
	t.Parallel()

	// LoadDefaultConfig succeeds offline (it does not validate credentials),
	// so the constructor returns a usable provider.
	provider, err := secrets.NewAWSProvider(context.Background(), testSecretARN)
	require.NoError(t, err)
	assert.NotNil(t, provider)
}
