package models_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// usNumber is the standard form of (305) 555-0100, the number the US-format
// cases below are written in.
var usNumber = models.ParsedPhone{ //nolint:gochecknoglobals // read-only expectation shared by table rows
	E164:               "+13055550100",
	International:      "+1 305-555-0100",
	RegionCode:         "US",
	CountryCallingCode: 1,
	NationalNumber:     "3055550100",
	LineType:           models.PhoneLineTypeFixedLineOrMobile,
}

// TestParsePhoneStandardizesHowPeopleWriteNumbers: one US number, written
// every way people type, paste, or export it, has one standard form.
func TestParsePhoneStandardizesHowPeopleWriteNumbers(t *testing.T) {
	t.Parallel()

	inputs := map[string]string{
		"digits only":                            "3055550100",
		"dashes":                                 "305-555-0100",
		"dots":                                   "305.555.0100",
		"spaces":                                 "305 555 0100",
		"parenthesized area code":                "(305) 555-0100",
		"parentheses without a space":            "(305)555-0100",
		"a slash":                                "305/555-0100",
		"square brackets":                        "[305] 555-0100",
		"a stray closing parenthesis":            "(305) 555-0100)",
		"the trunk 1 with dashes":                "1-305-555-0100",
		"the trunk 1 with parentheses":           "1 (305) 555-0100",
		"the trunk 1 with dots":                  "1.305.555.0100",
		"the trunk 1, digits only":               "13055550100",
		"the country code with spaces":           "+1 305 555 0100",
		"the country code with dashes":           "+1-305-555-0100",
		"the country code with dots":             "+1.305.555.0100",
		"the country code with parentheses":      "+1(305)555-0100",
		"E.164":                                  "+13055550100",
		"a doubled plus sign":                    "++13055550100",
		"surrounding spaces":                     "   305-555-0100   ",
		"a tab and a newline":                    "\t305-555-0100\n",
		"no-break spaces":                        "305 555 0100",
		"en dashes":                              "305–555–0100",
		"em dashes":                              "305—555—0100",
		"minus signs":                            "305−555−0100",
		"full-width digits":                      "３０５-５５５-０１００",
		"Arabic-Indic digits":                    "٣٠٥٥٥٥٠١٠٠",
		"a tel: link":                            "tel:+1-305-555-0100",
		"a TEL: link in capitals":                "TEL:+13055550100",
		"an sms: link":                           "sms:+13055550100",
		"a callto: link":                         "callto:+13055550100",
		"a label":                                "Phone: (305) 555-0100",
		"a label and a dash":                     "Cell - 305.555.0100",
		"the US international prefix, to the US": "011 1 305 555 0100",
	}

	for name, raw := range inputs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ParsePhone(raw, models.WithDefaultRegion("US"), models.RequireValidNumber())

			require.NoError(t, err)
			assert.Equal(t, usNumber, got)
		})
	}
}

// TestParsePhoneInternationalNumbers: numbers from other regions, written
// with their country code, with a trunk (0), with a region's own dialing
// prefix, or nationally in their own default region.
func TestParsePhoneInternationalNumbers(t *testing.T) {
	t.Parallel()

	london := models.ParsedPhone{
		E164: "+442079460000", International: "+44 20 7946 0000", RegionCode: "GB",
		CountryCallingCode: 44, NationalNumber: "2079460000", LineType: models.PhoneLineTypeFixedLine,
	}
	paris := models.ParsedPhone{
		E164: "+33612345678", International: "+33 6 12 34 56 78", RegionCode: "FR",
		CountryCallingCode: 33, NationalNumber: "612345678", LineType: models.PhoneLineTypeMobile,
	}
	tokyo := models.ParsedPhone{
		E164: "+81312345678", International: "+81 3-1234-5678", RegionCode: "JP",
		CountryCallingCode: 81, NationalNumber: "312345678", LineType: models.PhoneLineTypeFixedLine,
	}

	cases := []struct {
		name string
		raw  string
		opts []models.PhoneOption
		want models.ParsedPhone
	}{
		{name: "GB with its country code", raw: "+44 20 7946 0000", want: london},
		{name: "GB with the trunk (0)", raw: "+44 (0)20 7946 0000", want: london},
		{name: "GB nationally", raw: "020 7946 0000", opts: []models.PhoneOption{models.WithDefaultRegion("GB")}, want: london},
		{name: "GB from GB's 00 prefix", raw: "0044 20 7946 0000", opts: []models.PhoneOption{models.WithDefaultRegion("GB")}, want: london},
		{name: "GB from the US 011 prefix", raw: "011 44 20 7946 0000", opts: []models.PhoneOption{models.WithDefaultRegion("US")}, want: london},
		{name: "FR with its country code", raw: "+33 6 12 34 56 78", want: paris},
		{name: "FR nationally", raw: "06 12 34 56 78", opts: []models.PhoneOption{models.WithDefaultRegion("FR")}, want: paris},
		{name: "FR with dots", raw: "+33.6.12.34.56.78", want: paris},
		{name: "JP with its country code", raw: "+81 3-1234-5678", want: tokyo},
		{name: "JP nationally, with long-vowel dashes", raw: "03ー1234ー5678", opts: []models.PhoneOption{models.WithDefaultRegion("JP")}, want: tokyo},
		{
			name: "DE with the trunk (0)", raw: "+49 (0)30 12345678",
			want: models.ParsedPhone{
				E164: "+493012345678", International: "+49 30 12345678", RegionCode: "DE",
				CountryCallingCode: 49, NationalNumber: "3012345678", LineType: models.PhoneLineTypeFixedLine,
			},
		},
		{
			name: "IN mobile", raw: "+91 98765 43210",
			want: models.ParsedPhone{
				E164: "+919876543210", International: "+91 98765 43210", RegionCode: "IN",
				CountryCallingCode: 91, NationalNumber: "9876543210", LineType: models.PhoneLineTypeMobile,
			},
		},
		{
			name: "AU mobile", raw: "+61 412 345 678",
			want: models.ParsedPhone{
				E164: "+61412345678", International: "+61 412 345 678", RegionCode: "AU",
				CountryCallingCode: 61, NationalNumber: "412345678", LineType: models.PhoneLineTypeMobile,
			},
		},
		{
			name: "BR mobile", raw: "+55 11 91234-5678",
			want: models.ParsedPhone{
				E164: "+5511912345678", International: "+55 11 91234-5678", RegionCode: "BR",
				CountryCallingCode: 55, NationalNumber: "11912345678", LineType: models.PhoneLineTypeMobile,
			},
		},
		{
			name: "a non-geographic toll-free number", raw: "+800 1234 5678",
			want: models.ParsedPhone{
				E164: "+80012345678", International: "+800 1234 5678", RegionCode: "001",
				CountryCallingCode: 800, NationalNumber: "12345678", LineType: models.PhoneLineTypeTollFree,
			},
		},
		{
			name: "a US toll-free number", raw: "1 (800) 234-5678", opts: []models.PhoneOption{models.WithDefaultRegion("US")},
			want: models.ParsedPhone{
				E164: "+18002345678", International: "+1 800-234-5678", RegionCode: "US",
				CountryCallingCode: 1, NationalNumber: "8002345678", LineType: models.PhoneLineTypeTollFree,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ParsePhone(tc.raw, append(tc.opts, models.RequireValidNumber())...)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestParsePhoneRefusesWhatIsWrong: input that isn't one phone number is
// refused with a fixed message, whatever sanitizing leaves of it, and no
// error includes the input.
func TestParsePhoneRefusesWhatIsWrong(t *testing.T) {
	t.Parallel()

	const (
		invalid     = "is not a valid phone number"
		extension   = "must not have an extension"
		countryCode = "must start with + and its country code"
	)

	us := []models.PhoneOption{models.WithDefaultRegion("US")}
	cases := []struct {
		name    string
		raw     string
		opts    []models.PhoneOption
		message string
	}{
		{name: "empty", raw: "", opts: us, message: "is required"},
		{name: "whitespace only", raw: " \t\n ", opts: us, message: "is required"},
		{name: "N/A", raw: "N/A", opts: us, message: invalid},
		{name: "a word", raw: "none", opts: us, message: invalid},
		{name: "punctuation only", raw: "+--()", opts: us, message: invalid},
		{name: "a zero", raw: "0", opts: us, message: invalid},
		{name: "too few digits", raw: "555-01", opts: us, message: invalid},
		{name: "nine digits", raw: "305-555-010", opts: us, message: invalid},
		{name: "eleven digits without the trunk 1", raw: "305-555-01000", opts: us, message: invalid},
		{name: "more than E.164's 15 digits", raw: "+12345678901234567", message: invalid},
		{name: "more digits than E.164 allows, though its region's metadata allows them", raw: "+81000000000000000", message: invalid},
		{name: "two numbers, comma-separated", raw: "+1 305 555 0100, +1 305 555 0101", opts: us, message: invalid},
		{name: "two numbers, slash-separated", raw: "305-555-0100 / 305-555-0101", opts: us, message: invalid},
		{name: "a second plus sign inside", raw: "+1+305", opts: us, message: invalid},
		{name: "a label holding a digit", raw: "Phone 2: 305-555-0100", opts: us, message: invalid},
		{name: "a letter O typed for a zero", raw: "305-555-O1OO", opts: us, message: invalid},
		{name: "a local number without its area code", raw: "555-0100", opts: us, message: invalid},
		{name: "a trunk 0 kept through an international prefix", raw: "011 82 0000000000", opts: us, message: invalid},
		{name: "keypad letters without KeypadLetters", raw: "1-800-FLOWERS", opts: us, message: invalid},
		{name: "an x extension", raw: "305-555-0100 x123", opts: us, message: extension},
		{name: "an ext. extension", raw: "305-555-0100 ext. 7", opts: us, message: extension},
		{name: "an extension spelled out", raw: "305-555-0100 extension 7", opts: us, message: extension},
		{name: "a # extension", raw: "+1 305 555 0100 #123", opts: us, message: extension},
		{name: "an RFC 3966 extension", raw: "tel:+1-305-555-0100;ext=123", opts: us, message: extension},
		{name: "an auto-dialing extension", raw: "(305) 555-0100,,123", opts: us, message: extension},
		{name: "a national number without a default region", raw: "305-555-0100", message: countryCode},
		{name: "a parenthesized national number without a default region", raw: "(305) 555-0100", message: countryCode},
		{name: "input far longer than a phone number", raw: "+1 305 555 0100 " + strings.Repeat("0", 60), opts: us, message: "is too long to be a phone number"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := models.ParsePhone(tc.raw, tc.opts...)

			require.ErrorIs(t, err, models.ErrValidation)

			var ve *models.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, fieldPhone, ve.Field)
			assert.Equal(t, tc.message, ve.Message)
			if strings.TrimSpace(tc.raw) != "" {
				assert.NotContains(t, err.Error(), strings.TrimSpace(tc.raw), "the error never includes the input")
			}
		})
	}
}

// TestParsePhoneRequireValidNumber: placeholder and made-up numbers have a
// possible length, so they pass the default check and fail RequireValidNumber.
func TestParsePhoneRequireValidNumber(t *testing.T) {
	t.Parallel()

	us := models.WithDefaultRegion("US")
	for _, raw := range []string{"000-000-0000", "123-456-7890", "555-555-5555", "111-111-1111", "(555) 123-4567"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			lenient, err := models.ParsePhone(raw, us)
			require.NoError(t, err, "a possible length passes the default check")
			assert.Equal(t, models.PhoneLineTypeUnknown, lenient.LineType)

			_, err = models.ParsePhone(raw, us, models.RequireValidNumber())
			var ve *models.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, "is not a number in use in its region", ve.Message)
		})
	}
}

// TestParsePhoneKeypadLetters: with KeypadLetters a lettered number reads as
// its keypad digits; an extension is still refused.
func TestParsePhoneKeypadLetters(t *testing.T) {
	t.Parallel()

	opts := []models.PhoneOption{models.WithDefaultRegion("US"), models.KeypadLetters()}

	got, err := models.ParsePhone("1-800-FLOWERS", opts...)
	require.NoError(t, err)
	assert.Equal(t, "+18003569377", got.E164)
	assert.Equal(t, models.PhoneLineTypeTollFree, got.LineType)

	_, err = models.ParsePhone("1-800-FLOWERS x12", opts...)
	var ve *models.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "must not have an extension", ve.Message)
}

// TestParsePhoneOptions: a region is trimmed and case-folded, nil options are
// ignored, and a region the metadata doesn't know is the caller's
// configuration error, not the input's.
func TestParsePhoneOptions(t *testing.T) {
	t.Parallel()

	got, err := models.ParsePhone("305-555-0100", models.WithDefaultRegion(" us "), nil)
	require.NoError(t, err)
	assert.Equal(t, usNumber, got)

	_, err = models.ParsePhone("+13055550100", models.WithDefaultRegion("XX"))
	require.ErrorIs(t, err, models.ErrUnknownPhoneRegion)
	assert.NotErrorIs(t, err, models.ErrValidation)
}

// TestParsePhoneOutputIsStable: either standard form parses back to the same
// number, with no default region.
func TestParsePhoneOutputIsStable(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"(305) 555-0100", "+44 (0)20 7946 0000", "+81 3-1234-5678", "+800 1234 5678"} {
		got, err := models.ParsePhone(raw, models.WithDefaultRegion("US"))
		require.NoError(t, err)

		for _, form := range []string{got.E164, got.International} {
			again, againErr := models.ParsePhone(form)
			require.NoError(t, againErr, form)
			assert.Equal(t, got, again, form)
		}
	}
}

// BenchmarkParsePhone measures sanitizing and parsing a formatted national
// number against its region's numbering plan.
func BenchmarkParsePhone(b *testing.B) {
	b.ReportAllocs()

	us := models.WithDefaultRegion("US")
	for range b.N {
		_, _ = models.ParsePhone("(305) 555-0100", us)
	}
}

// FuzzParsePhone proves ParsePhone never panics on arbitrary input; that what
// it accepts is E.164 that parses back to the same number; and that
// RequireValidNumber only narrows what it accepts.
func FuzzParsePhone(f *testing.F) {
	for _, seed := range []string{
		"(305) 555-0100",
		"+1 202 555 0143",
		"1-800-FLOWERS",
		"305-555-0100 x123",
		"+800 1234 5678",
		"",
		"+",
		"sms:+15613334444",
		"+44 (0)20 7946 0000",
		"٣٠٥٥٥٥٠١٠٠",
		"00000000000000000000",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		us := models.WithDefaultRegion("US")

		got, err := models.ParsePhone(raw, us)
		if err != nil {
			return
		}
		assertStandardPhone(t, raw, got)
		if valid, validErr := models.ParsePhone(raw, us, models.RequireValidNumber()); validErr == nil {
			assert.Equal(t, got, valid, "RequireValidNumber changed the result for %q", raw)
		}
	})
}

// assertStandardPhone fails unless got, which ParsePhone made of raw, is
// E.164 that parses back to the same number.
func assertStandardPhone(t *testing.T, raw string, got models.ParsedPhone) {
	t.Helper()

	require.Regexp(t, e164Test, got.E164, "%q was accepted as something other than E.164", raw)

	again, err := models.ParsePhone(got.E164)
	require.NoError(t, err, "the E.164 of %q is refused", raw)
	assert.Equal(t, got, again, "the E.164 of %q parses back to the same number", raw)
}
