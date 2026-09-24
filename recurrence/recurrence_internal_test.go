package recurrence

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// sscanfParseTimeHHMM is the previous fmt.Sscanf-based implementation, kept as a
// differential reference so FuzzParseTimeHHMM can prove the hand-rolled parser
// preserves its exact acceptance semantics.
func sscanfParseTimeHHMM(s string) (int, int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, 0, fmt.Errorf("%w: %s: %w", ErrInvalidTimeFormat, s, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("%w: %s", ErrTimeOutOfRange, s)
	}
	return h, m, nil
}

// FuzzParseTimeHHMM asserts the hand-rolled parseTimeHHMM accepts/rejects (and,
// on success, decodes) exactly what the fmt.Sscanf reference does.
func FuzzParseTimeHHMM(f *testing.F) {
	seeds := []string{
		"17:00", "9:5", "09:05", "17:00:00", "17:0abc", " 17:00", "17: 00",
		"17 :00", "+17:+05", "-1:00", "25:61", "abc", "1:", ":5", "17",
		"0:\n0", "9\r\n:5", "9:\r\n5", "\t9:5", "9:5 ", " 9:5",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		gotH, gotM, gotErr := parseTimeHHMM(s)
		wantH, wantM, wantErr := sscanfParseTimeHHMM(s)

		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("error disagreement for %q: got %v, sscanf %v", s, gotErr, wantErr)
		}
		if gotErr == nil && (gotH != wantH || gotM != wantM) {
			t.Fatalf("value disagreement for %q: got (%d,%d), sscanf (%d,%d)", s, gotH, gotM, wantH, wantM)
		}
	})
}

// BenchmarkParseTimeHHMM measures the hot-path time parse used by
// NextOccurrence.
func BenchmarkParseTimeHHMM(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_, _, _ = parseTimeHHMM("17:30")
	}
}

// BenchmarkParsePattern measures JSON decoding of a well-formed pattern.
func BenchmarkParsePattern(b *testing.B) {
	raw := json.RawMessage(`{"day_of_week":"friday","start_time":"17:00","end_time":"22:00"}`)

	b.ReportAllocs()

	for range b.N {
		_, _ = ParsePattern(raw)
	}
}

// BenchmarkNextOccurrence measures the full parse-and-compute path callers hit
// when scheduling the next instance of a recurring event.
func BenchmarkNextOccurrence(b *testing.B) {
	raw := json.RawMessage(`{"day_of_week":"friday","start_time":"17:00","end_time":"22:00"}`)
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for range b.N {
		_, _ = NextOccurrence(raw, after, time.UTC)
	}
}
