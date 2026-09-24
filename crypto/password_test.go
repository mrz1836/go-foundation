package crypto_test

import (
	"strings"
	"testing"
	"testing/iotest"

	"github.com/mrz1836/go-foundation/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cheapParams keeps argon2id fast enough for unit tests while still exercising
// the real code path.
func cheapParams() crypto.Argon2Params {
	return crypto.Argon2Params{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	encoded, err := crypto.HashPasswordWithParams("correct horse", cheapParams())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v="))

	ok, err := crypto.VerifyPassword("correct horse", encoded)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = crypto.VerifyPassword("wrong horse", encoded)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHashPasswordUsesUniqueSalt(t *testing.T) {
	t.Parallel()

	a, err := crypto.HashPasswordWithParams("pw", cheapParams())
	require.NoError(t, err)

	b, err := crypto.HashPasswordWithParams("pw", cheapParams())
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
}

func TestHashPasswordDefaultParams(t *testing.T) {
	t.Parallel()

	encoded, err := crypto.HashPassword("pw")
	require.NoError(t, err)

	ok, err := crypto.VerifyPassword("pw", encoded)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestHashPasswordCSPRNGFailure covers the fatal salt-generation path: when the
// entropy source fails, hashing must return the wrapped error and no hash. It
// swaps the package entropy source, so it must not run in parallel.
func TestHashPasswordCSPRNGFailure(t *testing.T) {
	restore := crypto.SetRandReader(iotest.ErrReader(errRandFail))
	defer restore()

	encoded, err := crypto.HashPasswordWithParams("pw", cheapParams())
	require.ErrorIs(t, err, errRandFail)
	assert.Contains(t, err.Error(), "crypto: reading csprng")
	assert.Empty(t, encoded)
}

func TestVerifyPasswordMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
	}{
		{"empty", ""},
		{"wrong algorithm", "$bcrypt$v=19$m=8,t=1,p=1$c2FsdA$aGFzaA"},
		{"too few parts", "$argon2id$v=19$m=8,t=1,p=1$salt"},
		{"bad version token", "$argon2id$vX$m=8,t=1,p=1$c2FsdA$aGFzaA"},
		{"bad params token", "$argon2id$v=19$mX$c2FsdA$aGFzaA"},
		{"bad salt base64", "$argon2id$v=19$m=8,t=1,p=1$!!!$aGFzaA"},
		{"bad hash base64", "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ok, err := crypto.VerifyPassword("pw", tt.encoded)
			assert.False(t, ok)
			require.ErrorIs(t, err, crypto.ErrInvalidPasswordHash)
		})
	}
}

func TestVerifyPasswordIncompatibleVersion(t *testing.T) {
	t.Parallel()

	ok, err := crypto.VerifyPassword("pw", "$argon2id$v=18$m=8,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2g")
	assert.False(t, ok)
	require.ErrorIs(t, err, crypto.ErrIncompatiblePasswordHash)
}

// BenchmarkHashPassword documents argon2id's deliberate cost: "cheap" uses the
// reduced test parameters, "default" uses the shipped OWASP params. Unlike the
// HMAC hot path this is intentionally slow — it is reserved for low-entropy
// secrets, and high-entropy keys never pay it.
func BenchmarkHashPassword(b *testing.B) {
	cases := []struct {
		name string
		p    crypto.Argon2Params
	}{
		{"cheap", cheapParams()},
		{"default", crypto.DefaultArgon2Params()},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				if _, err := crypto.HashPasswordWithParams("correct horse", tc.p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerifyPassword measures the verify path (decode + one argon2id
// derivation + constant-time compare) at the cheap test parameters.
func BenchmarkVerifyPassword(b *testing.B) {
	encoded, err := crypto.HashPasswordWithParams("correct horse", cheapParams())
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := crypto.VerifyPassword("correct horse", encoded); err != nil {
			b.Fatal(err)
		}
	}
}
