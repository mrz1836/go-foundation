package crypto

import "errors"

// Static error variables for err113 compliance.
var (
	// ErrInvalidTokenLength is returned when a non-positive byte length is
	// requested from a token generator.
	ErrInvalidTokenLength = errors.New("crypto: token length must be positive")

	// ErrInvalidPasswordHash is returned when an encoded argon2id string cannot be
	// parsed (wrong shape, bad base64, or out-of-range parameters).
	ErrInvalidPasswordHash = errors.New("crypto: malformed argon2id hash")

	// ErrIncompatiblePasswordHash is returned when an encoded argon2id string
	// declares an argon2 version this build cannot verify.
	ErrIncompatiblePasswordHash = errors.New("crypto: incompatible argon2id version")
)
