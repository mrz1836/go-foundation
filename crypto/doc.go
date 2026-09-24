// Package crypto provides a small, opinionated set of cryptographic primitives
// for building authentication systems. It deliberately exposes a narrow surface
// so callers cannot reach for a weaker construction:
//
//   - RandomBytes / RandomToken / RandomTokenHex mint cryptographically-secure
//     opaque tokens (session tokens, magic-link tokens, nonces). The default
//     TokenBytes is 32 (256 bits of entropy).
//   - HMACSHA256 / HMACHex / HMACEqual are the keyed hash used for HIGH-entropy
//     secrets (for example API keys) and for integrity tags. HMAC gives O(1),
//     constant-length, direct-lookup validation with a constant-time compare —
//     no per-request password-hashing cost on a hot path.
//   - SHA256Hex derives non-reversible keys from opaque tokens so a raw token is
//     never used as (or recoverable from) a cache key.
//   - HashPassword / VerifyPassword implement argon2id, reserved strictly for
//     LOW-entropy secrets (human passphrases, recovery codes). It is never used
//     to validate high-entropy keys.
//
// Every comparison in this package is constant time; malformed inputs compare as
// not-equal or return an error rather than panicking.
package crypto
