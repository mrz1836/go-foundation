package models_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// janeAddress is the standard form of the address most cases below write.
const janeAddress = "jane@example.com"

// TestNormalizeEmailStandardizesHowPeopleWriteAddresses: addresses typed,
// pasted, or exported in the usual ways have one standard form, with the
// strictest options set.
func TestNormalizeEmailStandardizesHowPeopleWriteAddresses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, raw, wantAddress, wantRoot string
	}{
		{name: "mixed case", raw: "Jane.Doe@Example.COM", wantAddress: "jane.doe@example.com"},
		{name: "all capitals", raw: "JANE@EXAMPLE.COM", wantAddress: janeAddress},
		{name: "surrounding spaces", raw: "   jane@example.com   ", wantAddress: janeAddress},
		{name: "a tab and a newline", raw: "\tjane@example.com\n", wantAddress: janeAddress},
		{name: "a trailing no-break space", raw: "jane@example.com ", wantAddress: janeAddress},
		{name: "a mailto: link", raw: "mailto:jane@example.com", wantAddress: janeAddress},
		{name: "a MAILTO: link in capitals", raw: "MAILTO:Jane@Example.com", wantAddress: janeAddress},
		{name: "a mailto: link with a space", raw: "mailto: jane@example.com", wantAddress: janeAddress},
		{name: "angle brackets", raw: "<jane@example.com>", wantAddress: janeAddress},
		{name: "angle brackets with inner spaces", raw: "< jane@example.com >", wantAddress: janeAddress},
		{name: "a mailto: link in angle brackets", raw: "<mailto:jane@example.com>", wantAddress: janeAddress},
		{name: "straight double quotes", raw: `"jane@example.com"`, wantAddress: janeAddress},
		{name: "straight single quotes", raw: "'jane@example.com'", wantAddress: janeAddress},
		{name: "curly double quotes", raw: "“jane@example.com”", wantAddress: janeAddress},
		{name: "double quotes around an apostrophe", raw: `"O'Brien@example.ie"`, wantAddress: "o'brien@example.ie"},
		{name: "curly single quotes", raw: "‘jane@example.com’", wantAddress: janeAddress},
		{name: "a trailing comma from a list", raw: "jane@example.com,", wantAddress: janeAddress},
		{name: "a trailing semicolon from a list", raw: "jane@example.com;", wantAddress: janeAddress},
		{name: "angle brackets and a trailing comma", raw: "<jane@example.com>, ", wantAddress: janeAddress},
		{name: "full-width letters", raw: "ｊａｎｅ@ｅｘａｍｐｌｅ.ｃｏｍ", wantAddress: janeAddress},
		{name: "an accented local part in capitals", raw: "JÖHN@example.com", wantAddress: "jöhn@example.com"},
		{name: "an internationalized domain", raw: "jane@exämple.com", wantAddress: "jane@xn--exmple-cua.com"},
		{name: "an apostrophe", raw: "O'Brien@example.ie", wantAddress: "o'brien@example.ie"},
		{name: "underscores, hyphens, and digits", raw: "First_Last-1@sub.example.co.uk", wantAddress: "first_last-1@sub.example.co.uk"},
		{name: "a plus tag, which the alias root drops", raw: "jane+news@example.com", wantAddress: "jane+news@example.com", wantRoot: janeAddress},
		{
			name: "a provider alias with a plus tag and dots", raw: "Jane.Doe+Tag@GoogleMail.com",
			wantAddress: "jane.doe+tag@gmail.com", wantRoot: "janedoe@gmail.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizeEmail(tc.raw, models.StrictEmail())

			require.NoError(t, err)
			assert.Equal(t, tc.wantAddress, got.Address)
			wantRoot := tc.wantRoot
			if wantRoot == "" {
				wantRoot = tc.wantAddress
			}
			assert.Equal(t, wantRoot, got.Root)
		})
	}
}

// TestNormalizeEmailRefusesWhatIsWrong: input that isn't one address is
// refused with a fixed message, whatever unwrapping leaves of it, and no
// error includes the input.
func TestNormalizeEmailRefusesWhatIsWrong(t *testing.T) {
	t.Parallel()

	const (
		invalid      = "is not a valid email address"
		badDomain    = "has an invalid domain"
		badLocalPart = "has an invalid local part"
	)

	cases := []struct {
		name, raw, message string
	}{
		{name: "empty", raw: "", message: "is required"},
		{name: "whitespace only", raw: " \t\n ", message: "is required"},
		{name: "empty angle brackets", raw: "<>", message: "is required"},
		{name: "a mailto: scheme alone", raw: "mailto:", message: "is required"},
		{name: "no @", raw: "jane", message: invalid},
		{name: "no domain", raw: "jane@", message: invalid},
		{name: "no local part", raw: "@example.com", message: invalid},
		{name: "two @ signs", raw: "jane@@example.com", message: invalid},
		{name: "a doubled dot in the domain", raw: "jane@example..com", message: invalid},
		{name: "a doubled dot in the local part", raw: "jane..doe@example.com", message: invalid},
		{name: "a leading dot", raw: ".jane@example.com", message: invalid},
		{name: "a dot before the @", raw: "jane.@example.com", message: invalid},
		{name: "a space in the local part", raw: "jane doe@example.com", message: invalid},
		{name: "a space in the domain", raw: "jane@exa mple.com", message: invalid},
		{name: "a display name", raw: "Jane Doe <jane@example.com>", message: invalid},
		{name: "a list, comma-separated", raw: "jane@example.com, joe@example.com", message: invalid},
		{name: "a list, semicolon-separated", raw: "jane@example.com; joe@example.com", message: invalid},
		{name: "two trailing commas", raw: "jane@example.com,,", message: invalid},
		{name: "doubled angle brackets", raw: "<<jane@example.com>>", message: invalid},
		{name: "unmatched wrappers", raw: "<jane@example.com\"", message: invalid},
		{name: "a comment", raw: "jane(work)@example.com", message: invalid},
		{name: "a trailing comment", raw: "jane@example.com (Jane)", message: invalid},
		{name: "a domain starting with a hyphen", raw: "jane@-example.com", message: badDomain},
		{name: "a domain literal", raw: "jane@[192.168.0.1]", message: badDomain},
		{name: "a domain of only a soft hyphen", raw: "jane@\u00ad", message: badDomain},
		{name: "a mailto: link with a query", raw: "mailto:jane@example.com?subject=Hi", message: badDomain},
		{name: "a zero-width space", raw: "jane\u200b@example.com", message: badLocalPart},
		{name: "a rune that folds to a semicolon", raw: "jane\u037edoe@example.com", message: badLocalPart},
		{name: "a single-label domain", raw: "jane@localhost", message: "must have a dot in its domain"},
		{name: "a trailing dot", raw: "jane@example.com.", message: "must not end with a dot"},
		{name: "a quoted local part", raw: `"jane doe"@example.com`, message: "must not have a quoted local part"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := models.NormalizeEmail(tc.raw, models.StrictEmail())

			require.ErrorIs(t, err, models.ErrValidation)

			var ve *models.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, fieldEmail, ve.Field)
			assert.Equal(t, tc.message, ve.Message)
			if raw := strings.TrimSpace(tc.raw); raw != "" {
				assert.NotContains(t, err.Error(), raw, "the error never includes the input")
			}
		})
	}
}

// TestNormalizeEmailUnwrappingKeepsValidAddresses: unwrapping never touches
// an address that is valid as written, quoted local parts and apostrophes
// included.
func TestNormalizeEmailUnwrappingKeepsValidAddresses(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`"jane doe"@example.com`,
		`"@"@example.com`,
		"'jane'@example.com",
		"o'brien@example.ie",
		janeAddress,
	} {
		got, err := models.NormalizeEmail(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, strings.ToLower(raw), got.Address, raw)
	}
}

// TestNormalizeEmailOutputIsStable: the standard form normalizes to itself.
func TestNormalizeEmailOutputIsStable(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"<Jane.Doe+Tag@GoogleMail.com>", "JÖHN@EXÄMPLE.COM", `"jane doe"@example.com`, "mailto:jane@example.com."} {
		got, err := models.NormalizeEmail(raw)
		require.NoError(t, err, raw)

		again, err := models.NormalizeEmail(got.Address)
		require.NoError(t, err, got.Address)
		assert.Equal(t, got.Address, again.Address, raw)
		assert.Equal(t, got.Root, again.Root, raw)
	}
}

// FuzzNormalizeEmailIsStable proves that whatever NormalizeEmail accepts, its
// Address normalizes to itself.
func FuzzNormalizeEmailIsStable(f *testing.F) {
	for _, seed := range []string{
		janeAddress,
		"<Jane@Example.COM>,",
		"mailto:jane@example.com",
		`"weird@local"@example.com`,
		"“jane@example.com”",
		"JÖHN@EXÄMPLE.COM",
		"Jane.Doe+Tag@GoogleMail.com",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got, err := models.NormalizeEmail(raw)
		if err != nil {
			return
		}

		again, err := models.NormalizeEmail(got.Address)
		require.NoError(t, err, "%q normalized to %q, which is refused", raw, got.Address)
		assert.Equal(t, got.Address, again.Address, "the address of %q normalizes to itself", raw)
		assert.Equal(t, got.Root, again.Root, "the root of %q normalizes to itself", raw)
	})
}
