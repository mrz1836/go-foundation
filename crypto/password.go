package crypto

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// maxDecodedLength bounds the salt/hash byte lengths accepted by decodeArgon2 so
// a hostile encoded value cannot request an absurd allocation and so the
// len->uint32 conversions below are provably in range.
const maxDecodedLength = 1024

// Argon2Params configures argon2id password hashing.
type Argon2Params struct {
	// Memory is the memory cost in KiB.
	Memory uint32
	// Iterations is the time cost (number of passes).
	Iterations uint32
	// Parallelism is the number of lanes.
	Parallelism uint8
	// SaltLength is the salt size in bytes used when hashing.
	SaltLength uint32
	// KeyLength is the derived-key size in bytes.
	KeyLength uint32
}

// DefaultArgon2Params returns argon2id parameters tuned per current OWASP
// guidance (19 MiB memory, 2 iterations, 1 lane). argon2id is reserved for
// LOW-entropy secrets (human passphrases, recovery codes); high-entropy keys use
// HMACHex instead and never pay this cost on a hot path.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      19 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// HashPassword hashes secret with argon2id using DefaultArgon2Params and a fresh
// random salt, returning a self-describing PHC-format string
// ($argon2id$v=19$m=,t=,p=$salt$hash) that VerifyPassword can parse.
func HashPassword(secret string) (string, error) {
	return HashPasswordWithParams(secret, DefaultArgon2Params())
}

// HashPasswordWithParams is HashPassword with caller-supplied cost parameters.
// It is exported so operators can tune cost per environment (and so tests can
// use cheap parameters).
func HashPasswordWithParams(secret string, p Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := io.ReadFull(randReader, salt); err != nil {
		return "", fmt.Errorf("crypto: reading csprng: %w", err)
	}

	hash := argon2.IDKey([]byte(secret), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	b64 := base64.RawStdEncoding

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		b64.EncodeToString(salt), b64.EncodeToString(hash),
	), nil
}

// VerifyPassword reports whether secret matches an encoded argon2id hash from
// HashPassword. The final comparison is constant time. A malformed or
// unsupported encoded value returns an error and never a false positive.
func VerifyPassword(secret, encoded string) (bool, error) {
	p, salt, hash, err := decodeArgon2(encoded)
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey([]byte(secret), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	return subtle.ConstantTimeCompare(hash, computed) == 1, nil
}

// decodeArgon2 parses a PHC-format argon2id string into its parameters, salt,
// and hash. It validates the algorithm, version, and lengths so callers can rely
// on the returned KeyLength being a safe uint32.
func decodeArgon2(encoded string) (Argon2Params, []byte, []byte, error) {
	// Shape: ["", "argon2id", "v=19", "m=..,t=..,p=..", <salt>, <hash>]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidPasswordHash
	}

	if err := checkArgon2Version(parts[2]); err != nil {
		return Argon2Params{}, nil, nil, err
	}

	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidPasswordHash
	}

	salt, err := decodeBounded(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}

	hash, err := decodeBounded(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}

	p.SaltLength = uint32(len(salt)) //nolint:gosec // len bounded to [1,maxDecodedLength] by decodeBounded
	p.KeyLength = uint32(len(hash))  //nolint:gosec // len bounded to [1,maxDecodedLength] by decodeBounded

	return p, salt, hash, nil
}

// checkArgon2Version parses the "v=<n>" segment and verifies it names an argon2
// version this build can verify.
func checkArgon2Version(part string) error {
	var version int
	if _, err := fmt.Sscanf(part, "v=%d", &version); err != nil {
		return ErrInvalidPasswordHash
	}

	if version != argon2.Version {
		return ErrIncompatiblePasswordHash
	}

	return nil
}

// decodeBounded base64-decodes a salt or hash segment and rejects an empty or
// implausibly large result, so the byte length is a safe non-zero uint32.
func decodeBounded(segment string) ([]byte, error) {
	b, err := base64.RawStdEncoding.DecodeString(segment)
	if err != nil || len(b) == 0 || len(b) > maxDecodedLength {
		return nil, ErrInvalidPasswordHash
	}

	return b, nil
}
