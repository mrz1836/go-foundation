package pagination_test

import (
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
