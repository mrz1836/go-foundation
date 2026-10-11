package pagination

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	// keysetVersion1 is the first byte of a version-1 keyset payload.
	keysetVersion1 = 0x01

	// legacyCursorLen is the payload length of a cursor from EncodeCursor. No
	// version-1 payload is this long: the shortest is 14 bytes.
	legacyCursorLen = 8

	// keysetHeaderLen is a version-1 payload's version byte, Unix seconds, and
	// nanoseconds.
	keysetHeaderLen = 13

	// maxCursorLen is the longest cursor string DecodeKeyset reads.
	maxCursorLen = 1024

	// stackPayloadLen is the payload EncodeKeyset and DecodeKeyset hold on the
	// stack: the header, a one-byte id length, and an id of up to 50 bytes (a
	// UUID included). A longer payload moves to the heap.
	stackPayloadLen = 64

	// nanosPerSecond bounds a version-1 payload's nanosecond field.
	nanosPerSecond = 1_000_000_000
)

// Keyset is a position in a list ordered by a time and an id. At is the
// position's time and ID its tie-breaker. Legacy marks a position read from a
// whole-second cursor (EncodeCursor): At is then that second, a lower bound on
// its row's time, and ID is empty. The zero Keyset is no position: the first
// page.
type Keyset struct {
	At     time.Time
	ID     string
	Legacy bool
}

// IsZero reports whether k is no position, the first page.
func (k Keyset) IsZero() bool {
	return k.At.IsZero() && k.ID == "" && !k.Legacy
}

// EncodeKeyset returns an opaque, URL-safe cursor for the position (at, id).
//
// The cursor is a version byte, the Unix seconds and nanoseconds of at, the
// id's length, and the id's bytes, as unpadded URL-safe base64. at is stored
// as an instant at full precision, whatever its zone. Encode the position of
// a page's last row, with its time as the database returned it: PostgreSQL
// keeps microseconds, so a finer in-memory time can sort after its own row.
// An id longer than 753 bytes makes a cursor longer than DecodeKeyset accepts.
func EncodeKeyset(at time.Time, id string) string {
	var buf [stackPayloadLen]byte // stack-allocated; append moves a longer payload to the heap

	b := append(buf[:0], keysetVersion1)
	b = binary.BigEndian.AppendUint64(b, uint64(at.Unix()))       //nolint:gosec // two's complement keeps every int64, and DecodeKeyset reads it back as one
	b = binary.BigEndian.AppendUint32(b, uint32(at.Nanosecond())) //nolint:gosec // a nanosecond is below 1,000,000,000, within uint32
	b = binary.AppendUvarint(b, uint64(len(id)))
	b = append(b, id...)

	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeKeyset reads a cursor written by EncodeKeyset, or by EncodeCursor as a
// legacy position.
//
// It accepts URL-safe and standard base64, padded or not, up to 1,024
// characters, and returns At in UTC. A cursor from EncodeCursor holds whole
// seconds only, so it decodes with Legacy set and At at its second, a lower
// bound on its row's time. Any other payload must be exactly what
// EncodeKeyset writes. Every refusal wraps ErrInvalidCursor and names the
// reason, never the cursor.
func DecodeKeyset(s string) (Keyset, error) {
	var buf [stackPayloadLen]byte // stack-allocated; only the id is copied out

	b, err := cursorBytes(buf[:], s)
	if err != nil {
		return Keyset{}, err
	}

	if len(b) == legacyCursorLen {
		return legacyKeyset(b), nil
	}

	return parseKeysetV1(b)
}

// cursorBytes decodes a cursor string's payload from any base64 form into
// dst, or into a larger buffer when dst is too small, and returns the decoded
// bytes.
func cursorBytes(dst []byte, s string) ([]byte, error) {
	if s == "" {
		return nil, invalidCursor("empty")
	}

	if len(s) > maxCursorLen {
		return nil, invalidCursor("longer than 1024 characters")
	}

	encodings := [2]*base64.Encoding{base64.RawURLEncoding, base64.RawStdEncoding}
	if strings.HasSuffix(s, "=") {
		encodings = [2]*base64.Encoding{base64.URLEncoding, base64.StdEncoding}
	}

	if need := base64.RawStdEncoding.DecodedLen(len(s)); need > len(dst) {
		dst = make([]byte, need)
	}

	for _, enc := range encodings {
		if n, err := enc.Decode(dst, []byte(s)); err == nil {
			if n == 0 {
				return nil, invalidCursor("empty")
			}

			return dst[:n], nil
		}
	}

	return nil, invalidCursor("bad encoding")
}

// legacyKeyset reads an EncodeCursor payload: whole Unix seconds.
func legacyKeyset(b []byte) Keyset {
	seconds := int64(binary.BigEndian.Uint64(b)) //nolint:gosec // EncodeCursor wrote an int64 as its two's complement

	return Keyset{At: time.Unix(seconds, 0).UTC(), Legacy: true}
}

// parseKeysetV1 reads a version-1 payload, refusing any shape EncodeKeyset
// can't write, so that one position has one cursor.
func parseKeysetV1(b []byte) (Keyset, error) {
	if b[0] != keysetVersion1 {
		return Keyset{}, invalidCursor("unknown version")
	}

	if len(b) < keysetHeaderLen {
		return Keyset{}, invalidCursor("truncated time")
	}

	seconds := int64(binary.BigEndian.Uint64(b[1:9])) //nolint:gosec // EncodeKeyset wrote an int64 as its two's complement
	nanos := binary.BigEndian.Uint32(b[9:keysetHeaderLen])

	if nanos >= nanosPerSecond {
		return Keyset{}, invalidCursor("nanoseconds out of range")
	}

	id, err := keysetID(b[keysetHeaderLen:])
	if err != nil {
		return Keyset{}, err
	}

	return Keyset{At: time.Unix(seconds, int64(nanos)).UTC(), ID: id}, nil
}

// keysetID reads a version-1 payload's id: a minimal uvarint length, then
// exactly that many bytes.
func keysetID(b []byte) (string, error) {
	length, n := binary.Uvarint(b)

	switch {
	case n == 0:
		return "", invalidCursor("truncated id length")
	case n < 0:
		return "", invalidCursor("id length overflows")
	case n > 1 && b[n-1] == 0:
		return "", invalidCursor("id length not minimal")
	}

	rest := b[n:]

	switch {
	case uint64(len(rest)) < length:
		return "", invalidCursor("truncated id")
	case uint64(len(rest)) > length:
		return "", invalidCursor("trailing bytes")
	}

	return string(rest), nil
}

// invalidCursor returns ErrInvalidCursor with a reason that never includes the
// cursor, since a cursor can carry a row's id.
func invalidCursor(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidCursor, reason)
}
