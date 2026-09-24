package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// SHA256Hex returns the lowercase hex SHA-256 of s. It is used to derive cache
// keys from opaque tokens (for example session:<sha256(token)>) so a raw token
// is never used as, or recoverable from, a cache key. SHA-256 here is a key
// derivation, not a credential comparison — never compare secrets by their
// SHA-256 without HMAC.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HMACSHA256 returns the raw HMAC-SHA256 of message under key.
func HMACSHA256(key, message []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)

	return mac.Sum(nil)
}

// HMACHex returns the lowercase hex HMAC-SHA256 of message under pepper. This is
// the keyed hash used for high-entropy secrets (for example API keys) and for
// integrity tags: it is O(1) to compute, constant length, and enables
// direct-lookup validation without a per-request password-hash cost.
func HMACHex(pepper []byte, message string) string {
	return hex.EncodeToString(HMACSHA256(pepper, []byte(message)))
}

// HMACEqual reports whether two hex-encoded HMAC tags are equal in constant
// time. A malformed hex input compares as not-equal and never panics, so it is
// safe to feed attacker-controlled values directly.
func HMACEqual(a, b string) bool {
	ab, err := hex.DecodeString(a)
	if err != nil {
		return false
	}

	bb, err := hex.DecodeString(b)
	if err != nil {
		return false
	}

	return hmac.Equal(ab, bb)
}

// ConstantTimeEqual reports whether a and b are equal in constant time. Use it
// to compare fixed-length opaque values (nonces, state tokens) where timing must
// not leak how much of a mismatch occurred.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
