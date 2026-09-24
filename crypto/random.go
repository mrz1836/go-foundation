package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// TokenBytes is the default entropy, in bytes, of a generated opaque token.
// 32 bytes is 256 bits, a common strength for session and magic-link tokens.
const TokenBytes = 32

// randReader is the entropy source for every CSPRNG read in this package. It
// defaults to crypto/rand.Reader in production; tests swap it (via the seam in
// export_test.go) to exercise the fatal read-failure paths that are otherwise
// unreachable. It is never reassigned outside tests.
//
//nolint:gochecknoglobals // deliberate, test-only injection seam
var randReader io.Reader = rand.Reader

// RandomBytes returns n cryptographically-secure random bytes. It returns
// ErrInvalidTokenLength for a non-positive n, and wraps any CSPRNG failure
// (which callers should treat as fatal — never fall back to a weaker source).
func RandomBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, ErrInvalidTokenLength
	}

	b := make([]byte, n)
	if _, err := io.ReadFull(randReader, b); err != nil {
		return nil, fmt.Errorf("crypto: reading csprng: %w", err)
	}

	return b, nil
}

// RandomToken returns a URL-safe, unpadded base64 token carrying n bytes of
// entropy. With n == TokenBytes the result is a 256-bit opaque token suitable
// for session cookies, magic-link tokens, and access tokens.
func RandomToken(n int) (string, error) {
	b, err := RandomBytes(n)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RandomTokenHex returns a lowercase hex token carrying n bytes of entropy. Hex
// is used where a value must be case-insensitive or embedded in a context that
// mangles URL-safe base64.
func RandomTokenHex(n int) (string, error) {
	b, err := RandomBytes(n)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
