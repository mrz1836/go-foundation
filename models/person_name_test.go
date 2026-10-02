package models_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/mrz1836/go-foundation/models"
)

// fieldPersonName is the error-field label used across the person-name cases.
const fieldPersonName = "given_name"

// The five messages NormalizePersonName returns. None includes the input.
const (
	msgPersonNameCharacter = "contains a character that is not allowed"
	msgPersonNameStart     = "must start with a letter or an apostrophe"
	msgPersonNameEnd       = "must not end with a hyphen"
	msgPersonNameLetter    = "must contain a letter"
	msgPersonNameTooLong   = "exceeds 100 characters"
)

// requireNameRefused asserts that NormalizePersonName refused its input with
// message: no value (never a stripped copy) and a *ValidationError for the
// field.
func requireNameRefused(t *testing.T, got string, err error, message string) {
	t.Helper()

	assert.Empty(t, got, "a refused name returns no value, never a stripped one")

	var fieldErr *models.ValidationError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, fieldPersonName, fieldErr.Field)
	assert.Equal(t, message, fieldErr.Message)
}

func TestNormalizePersonName_AcceptsInternationalNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"O'Neil",
		"O’Neil",
		"d'Arcy",
		"Jean-Luc",
		"Mary-Kate",
		"St. John",
		"Jr.",
		"J.",
		"'t Hooft",
		"’t Hooft",
		"Nguyễn",
		"Trần",
		"Zoë",
		"Łukasz",
		"Björk Guðmundsdóttir",
		"Ngũgĩ",
		"Siân",
		"Αλέξανδρος",
		"Анна-Мария",
		"王小明",  //nolint:gosmopolitan // intentional non-Latin name fixture
		"田中太郎", //nolint:gosmopolitan // intentional non-Latin name fixture
		"김민준",
		"محمد",
		"عبد الله",
		"יִצְחָק",
		"अनुष्का",
		"สมชาย",
		"สมศักดิ์",
		"Kaʻiulani",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.True(t, norm.NFC.IsNormalString(name), "the fixture must be written in NFC")

			got, err := models.NormalizePersonName(name, fieldPersonName)
			require.NoError(t, err)
			assert.Equal(t, name, got)
		})
	}
}

func TestNormalizePersonName_NormalizesSpacingAndComposition(t *testing.T) {
	t.Parallel()

	thirtyMarks := "a" + strings.Repeat("\u0301", 30)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "decomposed accent composes", in: "Jose\u0301", want: "Jos\u00e9"},
		{name: "spaces trimmed and collapsed", in: "  Mary   Ann  ", want: "Mary Ann"},
		{name: "spaced hyphen kept", in: "Jean - Luc", want: "Jean - Luc"},
		{name: "thirty stacked marks", in: thirtyMarks, want: norm.NFC.String(thirtyMarks)},
		{name: "empty", in: "", want: ""},
		{name: "spaces only", in: "   ", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizePersonName(tc.in, fieldPersonName)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	got, err := models.NormalizePersonName("Jose\u0301", fieldPersonName)
	require.NoError(t, err)
	assert.Equal(t, 4, utf8.RuneCountInString(got), "NFC composes e and the accent into one rune")

	got, err = models.NormalizePersonName(thirtyMarks, fieldPersonName)
	require.NoError(t, err)
	assert.Equal(t, "\u00e1"+strings.Repeat("\u0301", 29), got)
}

func TestNormalizePersonName_RejectsDisallowedCharacters(t *testing.T) {
	t.Parallel()

	characters := []struct {
		name string
		char string
	}{
		// bidi controls
		{name: "U+202E right-to-left override", char: "\u202e"},
		{name: "U+202A left-to-right embedding", char: "\u202a"},
		{name: "U+2066 left-to-right isolate", char: "\u2066"},
		{name: "U+200E left-to-right mark", char: "\u200e"},
		{name: "U+061C Arabic letter mark", char: "\u061c"},

		// zero-width characters
		{name: "U+200B zero-width space", char: "\u200b"},
		{name: "U+200C zero-width non-joiner", char: "\u200c"},
		{name: "U+200D zero-width joiner", char: "\u200d"},
		{name: "U+2060 word joiner", char: "\u2060"},
		{name: "U+FEFF zero-width no-break space", char: "\ufeff"},

		// letters and marks that render as nothing
		{name: "U+034F combining grapheme joiner", char: "\u034f"},
		{name: "U+115F Hangul choseong filler", char: "\u115f"},
		{name: "U+1160 Hangul jungseong filler", char: "\u1160"},
		{name: "U+17B4 Khmer inherent vowel aq", char: "\u17b4"},
		{name: "U+17B5 Khmer inherent vowel aa", char: "\u17b5"},
		{name: "U+180B Mongolian free variation selector one", char: "\u180b"},
		{name: "U+180F Mongolian free variation selector four", char: "\u180f"},
		{name: "U+3164 Hangul filler", char: "\u3164"},
		{name: "U+FE00 variation selector 1", char: "\ufe00"},
		{name: "U+FE0F variation selector 16", char: "\ufe0f"},
		{name: "U+FFA0 halfwidth Hangul filler", char: "\uffa0"},
		{name: "U+E0100 variation selector 17", char: "\U000e0100"},
		{name: "U+E01EF variation selector 256", char: "\U000e01ef"},

		// control characters
		{name: "U+0000 null", char: "\x00"},
		{name: "tab", char: "\t"},
		{name: "newline", char: "\n"},
		{name: "carriage return", char: "\r"},
		{name: "U+001F unit separator", char: "\x1f"},
		{name: "U+007F delete", char: "\x7f"},
		{name: "U+0085 next line", char: "\u0085"},

		// digits and numbers
		{name: "digit 0", char: "0"},
		{name: "digit 9", char: "9"},
		{name: "U+0661 Arabic-Indic digit one", char: "\u0661"},
		{name: "U+0967 Devanagari digit one", char: "\u0967"},
		{name: "U+FF11 fullwidth digit one", char: "\uff11"},
		{name: "U+2163 Roman numeral four", char: "\u2163"},
		{name: "U+00B2 superscript two", char: "\u00b2"},

		// markup, other punctuation, and symbols
		{name: "less-than", char: "<"},
		{name: "greater-than", char: ">"},
		{name: "ampersand", char: "&"},
		{name: "double quote", char: `"`},
		{name: "slash", char: "/"},
		{name: "backslash", char: `\`},
		{name: "at sign", char: "@"},
		{name: "underscore", char: "_"},
		{name: "comma", char: ","},
		{name: "semicolon", char: ";"},
		{name: "parenthesis", char: "("},
		{name: "U+2018 left single quotation mark", char: "\u2018"},
		{name: "U+2010 hyphen", char: "\u2010"},
		{name: "U+00AD soft hyphen", char: "\u00ad"},

		// other spaces, emoji, private use, and invalid UTF-8
		{name: "U+00A0 no-break space", char: "\u00a0"},
		{name: "U+3000 ideographic space", char: "\u3000"},
		{name: "U+2028 line separator", char: "\u2028"},
		{name: "U+1F600 emoji", char: "\U0001f600"},
		{name: "U+E000 private use", char: "\ue000"},
		{name: "invalid UTF-8", char: "\xff"},
	}

	// Trimming removes only ASCII spaces, so these are refused at either end
	// too.
	atEnds := map[string]bool{"\u202e": true, "\u200b": true, "\ufeff": true, "\t": true, "\n": true}

	for _, c := range characters {
		inputs := []string{"Ann" + c.char + "Lee"}
		if atEnds[c.char] {
			inputs = append(inputs, c.char+"Ann", "Ann"+c.char)
		}

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			for _, in := range inputs {
				got, err := models.NormalizePersonName(in, fieldPersonName)
				requireNameRefused(t, got, err, msgPersonNameCharacter)
			}
		})
	}

	// Letters that render as nothing pass every shape rule, so only the
	// character check refuses them as a whole name.
	for _, whole := range []struct {
		name string
		in   string
	}{
		{name: "U+3164 alone", in: "\u3164"},
		{name: "U+3164 doubled", in: "\u3164\u3164"},
		{name: "U+115F alone", in: "\u115f"},
		{name: "U+115F doubled", in: "\u115f\u115f"},
		{name: "U+FFA0 alone", in: "\uffa0"},
		{name: "U+FFA0 doubled", in: "\uffa0\uffa0"},
		// NFC inserts U+034F after the 30th consecutive mark.
		{name: "thirty-one stacked marks", in: "a" + strings.Repeat("\u0301", 31)},
	} {
		t.Run(whole.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizePersonName(whole.in, fieldPersonName)
			requireNameRefused(t, got, err, msgPersonNameCharacter)
		})
	}
}

func TestNormalizePersonName_RejectsInvalidShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		message string
	}{
		{name: "leading hyphen", in: "-Smith", message: msgPersonNameStart},
		{name: "leading full stop", in: ".John", message: msgPersonNameStart},
		{name: "leading combining mark", in: "\u0301a", message: msgPersonNameStart},
		{name: "trailing hyphen", in: "Smith-", message: msgPersonNameEnd},
		{name: "punctuation ending in a hyphen", in: "' -", message: msgPersonNameEnd},
		{name: "apostrophe only", in: "'", message: msgPersonNameLetter},
		{name: "punctuation only", in: "\u2019 . '", message: msgPersonNameLetter},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.NormalizePersonName(tc.in, fieldPersonName)
			requireNameRefused(t, got, err, tc.message)
		})
	}
}

func TestNormalizePersonName_CapsLengthAfterNormalization(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100, models.MaxPersonNameLength)

	han := "王" //nolint:gosmopolitan // intentional non-Latin name fixture

	for _, in := range []string{strings.Repeat("a", 100), strings.Repeat(han, 100)} {
		got, err := models.NormalizePersonName(in, fieldPersonName)
		require.NoError(t, err, "100 characters are accepted")
		assert.Equal(t, in, got)
	}

	for _, in := range []string{strings.Repeat("a", 101), strings.Repeat(han, 101)} {
		got, err := models.NormalizePersonName(in, fieldPersonName)
		requireNameRefused(t, got, err, msgPersonNameTooLong)
	}

	// 120 runes in, 60 after NFC: the cap counts the normalized result.
	got, err := models.NormalizePersonName(strings.Repeat("e\u0301", 60), fieldPersonName)
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("\u00e9", 60), got)
}

// BenchmarkNormalizePersonName measures the normalizer on a typical name with
// a hyphen, a space, and a typographic apostrophe.
func BenchmarkNormalizePersonName(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_, _ = models.NormalizePersonName("Jean-Luc O’Neil", fieldPersonName)
	}
}

// FuzzNormalizePersonName proves NormalizePersonName never panics, refuses
// with one of its fixed messages and no value, and otherwise only composes,
// trims, and collapses: every accepted result obeys the character and shape
// rules and normalizes to itself.
func FuzzNormalizePersonName(f *testing.F) {
	for _, seed := range []string{
		"O'Neil",
		"Jose\u0301",
		"  Mary   Ann  ",
		"Ann\u202eX",
		"Ann\u034fLee",
		"\u3164",
		"R2D2",
		"<b>",
		"",
		"\xff",
		strings.Repeat("a", 101),
		"'t Hooft",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got, err := models.NormalizePersonName(in, fieldPersonName)
		if err != nil {
			checkRefusedName(t, got, err)

			return
		}

		checkAcceptedName(t, in, got)
	})
}

// checkRefusedName fails t unless a refusal returned no value and a
// *ValidationError for the field with one of the five messages.
func checkRefusedName(t *testing.T, got string, err error) {
	t.Helper()

	if got != "" {
		t.Fatalf("refused input returned a value of %d bytes", len(got))
	}

	messages := []string{
		msgPersonNameCharacter, msgPersonNameStart, msgPersonNameEnd, msgPersonNameLetter, msgPersonNameTooLong,
	}

	var fieldErr *models.ValidationError
	if !errors.As(err, &fieldErr) || fieldErr.Field != fieldPersonName || !slices.Contains(messages, fieldErr.Message) {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

// checkAcceptedName fails t unless got is in, composed, trimmed, and collapsed,
// and nothing else, and obeys every rule.
func checkAcceptedName(t *testing.T, in, got string) {
	t.Helper()

	isSpace := func(r rune) bool { return r == ' ' }
	if want := strings.Join(strings.FieldsFunc(norm.NFC.String(in), isSpace), " "); got != want {
		t.Fatalf("NormalizePersonName(%q) = %q, want %q", in, got, want)
	}

	if got == "" {
		return
	}

	if broken := brokenPersonNameRule(got); broken != "" {
		t.Fatalf("accepted %q %s", got, broken)
	}

	if again, err := models.NormalizePersonName(got, fieldPersonName); err != nil || again != got {
		t.Fatalf("normalizing %q again gave %q, %v", got, again, err)
	}
}

// brokenPersonNameRule describes the first rule a non-empty accepted name
// breaks, or returns "" when it keeps them all.
func brokenPersonNameRule(name string) string {
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)

	switch {
	case strings.ContainsFunc(name, func(r rune) bool { return !allowedInPersonName(r) }):
		return "holds a disallowed rune"
	case !unicode.IsLetter(first) && first != '\'' && first != '\u2019':
		return "does not start with a letter or an apostrophe"
	case last == '-':
		return "ends with a hyphen"
	case !strings.ContainsFunc(name, unicode.IsLetter):
		return "has no letter"
	case utf8.RuneCountInString(name) > models.MaxPersonNameLength:
		return "is longer than the cap"
	}

	return ""
}

// allowedInPersonName is the character rule, restated independently of the
// implementation: a letter or a mark that does not render as nothing, or one of
// five punctuation runes.
func allowedInPersonName(r rune) bool {
	switch r {
	case ' ', '-', '\'', '\u2019', '.':
		return true
	}

	return (unicode.IsLetter(r) || unicode.IsMark(r)) &&
		!unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) &&
		!unicode.Is(unicode.Variation_Selector, r)
}
