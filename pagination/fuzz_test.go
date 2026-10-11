package pagination_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mrz1836/go-foundation/pagination"
)

func FuzzDecodeCursor(f *testing.F) {
	// Seed corpus with valid and edge-case inputs
	f.Add(pagination.EncodeCursor(time.Unix(0, 0)))
	f.Add("")
	f.Add("not-base64!!!")
	f.Add("AAAAAAAAAAA=")
	f.Add("////++++")

	f.Fuzz(func(t *testing.T, input string) {
		result, err := pagination.DecodeCursor(input)
		if err != nil {
			return // most random inputs are invalid — expected
		}

		// Roundtrip property: encode then decode should produce the same time
		encoded := pagination.EncodeCursor(result)

		decoded, err := pagination.DecodeCursor(encoded)
		if err != nil {
			t.Fatalf("roundtrip failed: EncodeCursor(%v) = %q, DecodeCursor returned error: %v",
				result, encoded, err)
		}

		if decoded.Unix() != result.Unix() {
			t.Errorf("roundtrip mismatch: got %v, want %v", decoded.Unix(), result.Unix())
		}
	})
}

func FuzzDecodeKeyset(f *testing.F) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 123456789, time.UTC)
	cursor := pagination.EncodeKeyset(at, "row-1")

	f.Add(cursor, at.Unix(), uint32(123456789), "row-1")
	f.Add(pagination.EncodeCursor(at), int64(0), uint32(0), "")
	f.Add("", int64(-1), uint32(999_999_999), "0192f3c4-7d1e-7a2b-8c3d-4e5f60718293")
	f.Add("!!!", int64(1<<62), uint32(1_000_000_000), "x")
	f.Add(cursor[:10], int64(-62135596800), uint32(1), "")

	f.Fuzz(func(t *testing.T, input string, seconds int64, nanos uint32, id string) {
		checkDecodedCursorReencodes(t, input)
		checkPositionRoundTrips(t, time.Unix(seconds, int64(nanos%1_000_000_000)), id)
	})
}

// checkDecodedCursorReencodes checks that any input decodes without a panic,
// and that a cursor that decodes re-encodes to the same position.
func checkDecodedCursorReencodes(t *testing.T, input string) {
	t.Helper()

	k, err := pagination.DecodeKeyset(input)
	if err != nil {
		return // most random inputs are invalid — expected
	}

	again, err := pagination.DecodeKeyset(pagination.EncodeKeyset(k.At, k.ID))
	if err != nil {
		t.Fatalf("re-encoding %+v: %v", k, err)
	}

	if !again.At.Equal(k.At) || again.ID != k.ID {
		t.Fatalf("re-encoding changed the position: got %+v, want %+v", again, k)
	}
}

// checkPositionRoundTrips checks that any position round-trips exactly, unless
// its id is too long to encode, which DecodeKeyset then refuses.
func checkPositionRoundTrips(t *testing.T, pos time.Time, id string) {
	t.Helper()

	got, err := pagination.DecodeKeyset(pagination.EncodeKeyset(pos, id))
	if len(id) > 753 {
		if !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("a %d-byte id: got %v, want ErrInvalidCursor", len(id), err)
		}

		return
	}

	if err != nil {
		t.Fatalf("round trip of (%v, %q): %v", pos, id, err)
	}

	if !got.At.Equal(pos) || got.ID != id || got.Legacy {
		t.Fatalf("round trip: got %+v, want (%v, %q)", got, pos, id)
	}
}
