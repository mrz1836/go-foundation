package crypto_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/mrz1836/go-foundation/crypto"
	"github.com/stretchr/testify/assert"
)

func TestSHA256Hex(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("hello"))
	want := hex.EncodeToString(sum[:])

	got := crypto.SHA256Hex("hello")
	assert.Equal(t, want, got)
	assert.Len(t, got, 64)
	assert.Equal(t, got, crypto.SHA256Hex("hello"))
	assert.NotEqual(t, got, crypto.SHA256Hex("world"))
}

func TestHMACSHA256(t *testing.T) {
	t.Parallel()

	got := crypto.HMACSHA256([]byte("k"), []byte("m"))
	assert.Len(t, got, sha256.Size)
	assert.Equal(t, got, crypto.HMACSHA256([]byte("k"), []byte("m")))
	assert.NotEqual(t, got, crypto.HMACSHA256([]byte("k2"), []byte("m")))
}

func TestHMACHexDeterministicAndKeyed(t *testing.T) {
	t.Parallel()

	pepper := []byte("pepper-1")
	tag := crypto.HMACHex(pepper, "msg")

	assert.Len(t, tag, 64)
	assert.Equal(t, tag, crypto.HMACHex(pepper, "msg"))                // deterministic
	assert.NotEqual(t, tag, crypto.HMACHex([]byte("pepper-2"), "msg")) // key-sensitive
	assert.NotEqual(t, tag, crypto.HMACHex(pepper, "other"))           // message-sensitive
}

func TestHMACEqual(t *testing.T) {
	t.Parallel()

	pepper := []byte("p")
	tag := crypto.HMACHex(pepper, "x")

	assert.True(t, crypto.HMACEqual(tag, tag))
	assert.True(t, crypto.HMACEqual(tag, crypto.HMACHex(pepper, "x")))
	assert.False(t, crypto.HMACEqual(tag, crypto.HMACHex(pepper, "y")))
	assert.False(t, crypto.HMACEqual(tag, "zzz-not-hex"))
	assert.False(t, crypto.HMACEqual("zzz-not-hex", tag))
	assert.False(t, crypto.HMACEqual("", tag))
}

func TestConstantTimeEqual(t *testing.T) {
	t.Parallel()

	assert.True(t, crypto.ConstantTimeEqual("abc", "abc"))
	assert.True(t, crypto.ConstantTimeEqual("", ""))
	assert.False(t, crypto.ConstantTimeEqual("abc", "abd"))
	assert.False(t, crypto.ConstantTimeEqual("abc", "abcd"))
}

// BenchmarkSHA256Hex sweeps small-to-large messages so a regression in the
// key-derivation path (used to derive cache keys from opaque tokens) shows up
// at each size.
func BenchmarkSHA256Hex(b *testing.B) {
	for _, size := range []int{16, 256, 4096} {
		msg := strings.Repeat("a", size)
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_ = crypto.SHA256Hex(msg)
			}
		})
	}
}

// BenchmarkHMACHex sweeps message sizes for the keyed hash used to validate
// high-entropy secrets; it is the authorizer hot path, so it must stay O(1)
// per byte and allocation-lean.
func BenchmarkHMACHex(b *testing.B) {
	pepper := []byte("benchmark-pepper-value")

	for _, size := range []int{16, 256, 4096} {
		msg := strings.Repeat("a", size)
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_ = crypto.HMACHex(pepper, msg)
			}
		})
	}
}

// BenchmarkHMACEqual measures the constant-time tag comparison on the equal
// (worst-case, full-scan) path.
func BenchmarkHMACEqual(b *testing.B) {
	pepper := []byte("benchmark-pepper-value")
	tag := crypto.HMACHex(pepper, "message")

	b.ReportAllocs()

	for range b.N {
		_ = crypto.HMACEqual(tag, tag)
	}
}
