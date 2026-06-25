package recurrence_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mrz1836/go-foundation/recurrence"
)

func FuzzParsePattern(f *testing.F) {
	// Seed corpus with valid and edge-case inputs.
	f.Add(`{"day_of_week":"friday","start_time":"17:00","end_time":"22:00"}`)
	f.Add(`{"day_of_week":"MONDAY","start_time":"00:00","end_time":"23:59"}`)
	f.Add(``)
	f.Add(`{not-json`)
	f.Add(`{}`)
	f.Add(`[]`)
	f.Add(`null`)

	f.Fuzz(func(t *testing.T, input string) {
		p, err := recurrence.ParsePattern(json.RawMessage([]byte(input)))
		if err != nil {
			return // malformed/empty inputs are expected to error
		}

		// Invariant: a successfully parsed pattern is never nil and its
		// day-of-week is normalized to lowercase.
		if p == nil {
			t.Fatalf("ParsePattern returned nil pattern with nil error for input %q", input)
		}
		if got := p.DayOfWeek; got != strings.ToLower(got) {
			t.Errorf("DayOfWeek not normalized to lowercase: %q", got)
		}
	})
}
