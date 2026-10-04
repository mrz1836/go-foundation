package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// fieldEmail is the error-field label of every email validation error.
const fieldEmail = "email"

// TestNormalizeEmailWithoutOptionsKeepsTheBroadSet pins the default: each form
// an option refuses is accepted without it.
func TestNormalizeEmailWithoutOptionsKeepsTheBroadSet(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ raw, wantAddress string }{
		"a quoted local part":   {raw: `"jane doe"@example.com`, wantAddress: `"jane doe"@example.com`},
		"a trailing root dot":   {raw: "jane@example.com.", wantAddress: janeAddress},
		"a single-label domain": {raw: "jane@localhost", wantAddress: "jane@localhost"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizeEmail(tc.raw)

			require.NoError(t, err)
			assert.Equal(t, tc.wantAddress, got.Address)
		})
	}
}

// TestNormalizeEmailOptionsRefuse covers each option and StrictEmail: the
// form each refuses, its fixed message, and that no error includes the input.
func TestNormalizeEmailOptionsRefuse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		opt     models.EmailOption
		message string
	}{
		{name: "RejectQuotedLocal", raw: `"jane doe"@example.com`, opt: models.RejectQuotedLocal(), message: "must not have a quoted local part"},
		{name: "RejectTrailingDot", raw: "jane@example.com.", opt: models.RejectTrailingDot(), message: "must not end with a dot"},
		{name: "RejectTrailingDot after whitespace", raw: " jane@example.com. ", opt: models.RejectTrailingDot(), message: "must not end with a dot"},
		{name: "RequireDottedDomain", raw: "jane@localhost", opt: models.RequireDottedDomain(), message: "must have a dot in its domain"},
		{name: "StrictEmail on a quoted local part", raw: `"jane doe"@example.com`, opt: models.StrictEmail(), message: "must not have a quoted local part"},
		{name: "StrictEmail on a trailing dot", raw: "jane@example.com.", opt: models.StrictEmail(), message: "must not end with a dot"},
		{name: "StrictEmail on a single-label domain", raw: "jane@localhost", opt: models.StrictEmail(), message: "must have a dot in its domain"},
		{name: "StrictEmail on a malformed address", raw: "jane@@example.com", opt: models.StrictEmail(), message: "is not a valid email address"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := models.NormalizeEmail(tc.raw, tc.opt)

			require.ErrorIs(t, err, models.ErrValidation)

			var ve *models.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, fieldEmail, ve.Field)
			assert.Equal(t, tc.message, ve.Message)
			assert.NotContains(t, err.Error(), "jane", "the error never includes the input")
		})
	}
}

// TestStrictEmailStillNormalizes: the strict options refuse forms, but an
// ordinary address is still trimmed, case-folded, and alias-collapsed as
// without them. Nil options are ignored.
func TestStrictEmailStillNormalizes(t *testing.T) {
	t.Parallel()

	got, err := models.NormalizeEmail("  Jane.Doe+news@GoogleMail.COM ", models.StrictEmail(), nil)

	require.NoError(t, err)
	assert.Equal(t, "jane.doe+news@gmail.com", got.Address)
	assert.Equal(t, "janedoe@gmail.com", got.Root)
	assert.Equal(t, "gmail.com", got.Domain)
	assert.False(t, got.IsQuoted)
}

// BenchmarkNormalizeEmailStrict measures the strict options' cost on the
// alias-rule path BenchmarkNormalizeEmail measures without them.
func BenchmarkNormalizeEmailStrict(b *testing.B) {
	b.ReportAllocs()

	strict := models.StrictEmail()
	for range b.N {
		_, _ = models.NormalizeEmail("Jane.Doe+newsletter@googlemail.com", strict)
	}
}

// FuzzNormalizeEmailStrict proves StrictEmail only narrows: whatever it
// accepts, NormalizeEmail accepts without it, with the same result.
func FuzzNormalizeEmailStrict(f *testing.F) {
	for _, seed := range []string{
		janeAddress,
		"  Jane@Example.COM  ",
		`"weird@local"@example.com`,
		"user@localhost",
		"user@example.com.",
		"user+tag@gmail.com",
		"@nope",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		strict, err := models.NormalizeEmail(raw, models.StrictEmail())
		if err != nil {
			return
		}

		broad, err := models.NormalizeEmail(raw)
		require.NoError(t, err, "StrictEmail accepted %q, which the broad set refuses", raw)
		assert.Equal(t, broad, strict, "StrictEmail changed the result for %q", raw)
	})
}
