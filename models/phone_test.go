package models_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// e164Test mirrors the canonical E.164 shape so the fuzz target can assert the
// invariant without reaching into the package's unexported pattern.
var e164Test = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// fieldPhone is the error-field label used across the phone test cases.
const fieldPhone = "phone"

// TestNormalizePhone exercises every branch of the normalizer: bare NANP
// 10-digit numbers, '+'-prefixed E.164 input, common human formatting, and the
// validation failures (empty, too short, too long, leading zero). The
// errorField argument is asserted so callers can shape errors to their own DTO.
func TestNormalizePhone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		raw        string
		errorField string
		want       string
		wantErr    bool
	}{
		// ── valid ────────────────────────────────────────────────────────
		{name: "bare 10-digit NANP", raw: "5551234567", errorField: fieldPhone, want: "+15551234567"},
		{name: "formatted NANP parens", raw: "(555) 123-4567", errorField: fieldPhone, want: "+15551234567"},
		{name: "dashed NANP", raw: "555-123-4567", errorField: fieldPhone, want: "+15551234567"},
		{name: "spaced NANP", raw: "555 123 4567", errorField: fieldPhone, want: "+15551234567"},
		{name: "plus-prefixed US", raw: "+1 555 123 4567", errorField: fieldPhone, want: "+15551234567"},
		{name: "plus-prefixed intl", raw: "+44 7911 123456", errorField: fieldPhone, want: "+447911123456"},
		{name: "plus-prefixed dotted", raw: "+1.555.123.4567", errorField: fieldPhone, want: "+15551234567"},
		{name: "surrounding whitespace", raw: "  5551234567  ", errorField: fieldPhone, want: "+15551234567"},
		{name: "non-NANP 11-digit no plus", raw: "445551234567", errorField: fieldPhone, want: "+445551234567"},

		// ── invalid ──────────────────────────────────────────────────────
		{name: "empty", raw: "", errorField: fieldPhone, wantErr: true},
		{name: "whitespace only", raw: "   ", errorField: fieldPhone, wantErr: true},
		{name: "too short", raw: "5", errorField: fieldPhone, wantErr: true},
		{name: "leading zero after plus", raw: "+0123456789", errorField: fieldPhone, wantErr: true},
		{name: "too long over 15 digits", raw: "+12345678901234567", errorField: fieldPhone, wantErr: true},
		{name: "letters only", raw: "abcdef", errorField: fieldPhone, wantErr: true},

		// ── custom error field propagation ───────────────────────────────
		{name: "custom field on empty", raw: "", errorField: "e164", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizePhone(tc.raw, tc.errorField)
			if tc.wantErr {
				require.Error(t, err, "input %q must be rejected", tc.raw)
				require.ErrorIs(t, err, models.ErrValidation, "error must wrap ErrValidation")
				assert.Contains(t, err.Error(), tc.errorField, "error must report the supplied field")

				return
			}

			require.NoError(t, err, "input %q must be accepted", tc.raw)
			assert.Equal(t, tc.want, got, "canonical E.164 mismatch")
		})
	}
}

// BenchmarkNormalizePhone measures the contact-dedup hot path on a typical
// formatted NANP input.
func BenchmarkNormalizePhone(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_, _ = models.NormalizePhone("(555) 123-4567", fieldPhone)
	}
}

// FuzzNormalizePhone proves NormalizePhone never panics on arbitrary input and
// that any successful result is valid E.164.
func FuzzNormalizePhone(f *testing.F) {
	for _, seed := range []string{
		"5551234567",
		"(555) 123-4567",
		"+44 7911 123456",
		"+0123",
		"",
		"   ",
		"abc",
		"+12345678901234567",
		"1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got, err := models.NormalizePhone(in, fieldPhone)
		if err != nil {
			return
		}

		if !e164Test.MatchString(got) {
			t.Fatalf("NormalizePhone(%q) = %q which is not valid E.164", in, got)
		}
	})
}
