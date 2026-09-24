package crypto_test

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"testing/iotest"

	"github.com/mrz1836/go-foundation/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRandFail is returned by the fake reader used to exercise the fatal CSPRNG
// read-failure paths. It is shared across the crypto_test package.
var errRandFail = errors.New("simulated csprng failure")

func TestRandomBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		n       int
		wantErr bool
		wantLen int
	}{
		{"zero rejected", 0, true, 0},
		{"negative rejected", -1, true, 0},
		{"single byte", 1, false, 1},
		{"token size", crypto.TokenBytes, false, 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := crypto.RandomBytes(tt.n)
			if tt.wantErr {
				require.ErrorIs(t, err, crypto.ErrInvalidTokenLength)
				assert.Nil(t, b)

				return
			}

			require.NoError(t, err)
			assert.Len(t, b, tt.wantLen)
		})
	}
}

// TestRandomBytesCSPRNGFailure covers the fatal read-failure path: a CSPRNG
// error must surface (wrapped) and never yield partial bytes. It swaps the
// package entropy source, so it must not run in parallel.
func TestRandomBytesCSPRNGFailure(t *testing.T) {
	restore := crypto.SetRandReader(iotest.ErrReader(errRandFail))
	defer restore()

	b, err := crypto.RandomBytes(crypto.TokenBytes)
	require.ErrorIs(t, err, errRandFail)
	assert.Contains(t, err.Error(), "crypto: reading csprng")
	assert.Nil(t, b)
}

func TestRandomBytesAreUnique(t *testing.T) {
	t.Parallel()

	a, err := crypto.RandomBytes(crypto.TokenBytes)
	require.NoError(t, err)

	b, err := crypto.RandomBytes(crypto.TokenBytes)
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
}

func TestRandomToken(t *testing.T) {
	t.Parallel()

	tok, err := crypto.RandomToken(crypto.TokenBytes)
	require.NoError(t, err)

	decoded, err := base64.RawURLEncoding.DecodeString(tok)
	require.NoError(t, err)
	assert.Len(t, decoded, crypto.TokenBytes)

	_, err = crypto.RandomToken(0)
	require.ErrorIs(t, err, crypto.ErrInvalidTokenLength)
}

func TestRandomTokenHex(t *testing.T) {
	t.Parallel()

	tok, err := crypto.RandomTokenHex(crypto.TokenBytes)
	require.NoError(t, err)

	decoded, err := hex.DecodeString(tok)
	require.NoError(t, err)
	assert.Len(t, decoded, crypto.TokenBytes)

	_, err = crypto.RandomTokenHex(-5)
	require.ErrorIs(t, err, crypto.ErrInvalidTokenLength)
}

// BenchmarkRandomToken sweeps token sizes so a regression in the CSPRNG read or
// the base64 encoding surfaces per size; TokenBytes (32) is the default.
func BenchmarkRandomToken(b *testing.B) {
	for _, size := range []int{16, crypto.TokenBytes, 64} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				if _, err := crypto.RandomToken(size); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
