package secrets

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
)

// Redacted is what a Value prints, logs, and marshals in place of its contents.
const Redacted = "[REDACTED]"

// Value holds a secret string that redacts itself. Printing it with any fmt
// verb, logging it with slog, and marshaling it to JSON, text, or XML all
// produce Redacted, including when the Value sits inside a struct, map, slice,
// or wrapped error. Only Reveal and Bytes return the contents, so every place
// that reads the secret is explicit and easy to find in a review.
//
// The zero Value is empty and ready to use. A Value is not comparable with ==,
// which would compare in variable time; use Equal.
type Value struct {
	_ [0]func() // makes Value non-comparable, so callers use the constant-time Equal

	// secret is held behind a pointer. fmt cannot call a method on a Value it
	// reaches through an unexported struct field, so it prints the Value's
	// fields by reflection instead; a pointer field prints as an address there,
	// never as the secret.
	secret *string
}

// NewValue returns a Value that holds s.
func NewValue(s string) Value {
	return Value{secret: &s}
}

// ValueFromBytes returns a Value that holds a copy of b. Later changes to b do
// not affect the Value, so the caller may zero b afterward.
func ValueFromBytes(b []byte) Value {
	return NewValue(string(b))
}

// Reveal returns the secret. Call it only where the secret is used, such as
// when building an Authorization header, and never to log or print it.
func (v Value) Reveal() string {
	if v.secret == nil {
		return ""
	}

	return *v.secret
}

// Bytes returns a copy of the secret. Changing the slice does not change the
// Value.
func (v Value) Bytes() []byte {
	return []byte(v.Reveal())
}

// IsZero reports whether the Value is empty.
func (v Value) IsZero() bool {
	return v.Reveal() == ""
}

// Equal reports whether v and other hold the same secret. It compares SHA-256
// digests in constant time, so the result does not reveal where two secrets
// first differ.
func (v Value) Equal(other Value) bool {
	a := sha256.Sum256([]byte(v.Reveal()))
	b := sha256.Sum256([]byte(other.Reveal()))

	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// String returns Redacted.
func (v Value) String() string {
	return Redacted
}

// GoString returns Redacted, so %#v never prints the secret.
func (v Value) GoString() string {
	return Redacted
}

// Format writes Redacted for every verb and flag, including %x, %q, and %d,
// which would otherwise format the underlying string.
func (v Value) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprint(f, Redacted)
}

// LogValue implements slog.LogValuer, so slog's text and JSON handlers log
// Redacted.
func (v Value) LogValue() slog.Value {
	return slog.StringValue(Redacted)
}

// MarshalJSON implements json.Marshaler and always encodes Redacted.
func (v Value) MarshalJSON() ([]byte, error) {
	return []byte(`"` + Redacted + `"`), nil
}

// MarshalText implements encoding.TextMarshaler and always encodes Redacted.
// encoding/xml and other text-based encoders use it.
func (v Value) MarshalText() ([]byte, error) {
	return []byte(Redacted), nil
}
