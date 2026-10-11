// Package pagination provides opaque, URL-safe cursors for keyset pagination.
//
// A keyset cursor (EncodeKeyset, DecodeKeyset) holds a position in a list
// ordered by a time and an id: the time at full precision and the id as a
// tie-breaker, after a version byte, so rows that share a second, or an exact
// time, page without a skip or a repeat. A cursor from EncodeCursor holds
// whole seconds only; DecodeKeyset reads it as a legacy position whose second
// is a lower bound on its row's time, so a page token issued before a switch
// to keyset cursors still resumes, possibly repeating that second's rows but
// never skipping one. models.WithKeyset pages a GORM query by a position.
package pagination

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

// ErrInvalidCursor is returned, wrapped, for any cursor that can't be decoded.
var ErrInvalidCursor = errors.New("pagination: invalid cursor")

// EncodeCursor encodes a time.Time to an opaque, URL-safe base64 cursor string.
// Internally stores the Unix timestamp (int64) as 8 big-endian bytes.
//
// The cursor keeps whole seconds only, so rows that share a second can't be
// told apart. Use EncodeKeyset for a position.
func EncodeCursor(t time.Time) string {
	var b [8]byte                                      // stack-allocated; avoids heap allocation per call
	binary.BigEndian.PutUint64(b[:], uint64(t.Unix())) //nolint:gosec // safe: Unix() fits in int64, conversion intentional

	return base64.URLEncoding.EncodeToString(b[:])
}

// DecodeCursor decodes an opaque cursor string back to a time.Time, in the
// local zone. It reads both a cursor from EncodeCursor and one from
// EncodeKeyset, whose id it drops. Use DecodeKeyset for a position.
// Returns an error wrapping ErrInvalidCursor for anything else, including a
// payload longer than 8 bytes that isn't a keyset cursor.
func DecodeCursor(s string) (time.Time, error) {
	k, err := DecodeKeyset(s)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(k.At.Unix(), int64(k.At.Nanosecond())), nil
}

// ListMeta contains count metadata attached to list responses.
// Embed or include in list response structs to expose total result counts.
type ListMeta struct {
	Total int `json:"total"`
}

// CursorPagination contains cursor-based pagination metadata for feed-style
// endpoints where offset pagination is impractical. Consumers use Cursor to
// fetch the next page and HasMore to decide whether to request one.
type CursorPagination struct {
	Cursor  string `json:"cursor,omitempty"`
	HasMore bool   `json:"has_more"`
	Limit   int    `json:"limit"`
}
