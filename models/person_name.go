package models

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxPersonNameLength is the longest name NormalizePersonName accepts, in
// Unicode code points of the normalized result.
const MaxPersonNameLength = 100

// The fixed messages of NormalizePersonName's errors. None includes the input.
const (
	msgNameCharacter = "contains a character that is not allowed"
	msgNameStart     = "must start with a letter or an apostrophe"
	msgNameEnd       = "must not end with a hyphen"
	msgNameLetter    = "must contain a letter"
	msgNameTooLong   = "exceeds 100 characters"
)

// NormalizePersonName validates and canonicalizes a person's name, in any
// script. It applies Unicode NFC, trims ASCII spaces (U+0020) from both ends,
// and collapses each run of spaces into one; it changes nothing else.
// Empty or space-only input returns "" and no error: whether a name is
// required is the caller's decision.
//
// A name may hold only letters and marks (Unicode categories L and M), except
// those that render as nothing (the Hangul fillers, the combining grapheme
// joiner, the Khmer inherent vowels, and the variation selectors), and the
// space, hyphen-minus, apostrophe, right single quotation mark (U+2019), and
// full stop. Anything else is refused, never stripped: control and format
// characters (bidi controls and zero-width characters included), other spaces,
// digits of every script, "<", ">", other punctuation and symbols, and invalid
// UTF-8. NFC follows the stream-safe format, which inserts U+034F after 30
// consecutive combining marks, so a name with more is refused too.
//
// The result must start with a letter or an apostrophe, must not end with a
// hyphen, must contain a letter, and may be at most MaxPersonNameLength code
// points long. A refused name returns "" and a *ValidationError for fieldName
// whose message never includes the input.
func NormalizePersonName(raw, fieldName string) (string, error) {
	name := strings.Trim(norm.NFC.String(raw), " ")
	if name == "" {
		return "", nil
	}

	if strings.ContainsFunc(name, func(r rune) bool { return !allowedNameRune(r) }) {
		return "", NewValidationError(fieldName, msgNameCharacter)
	}

	name = strings.Join(strings.FieldsFunc(name, isNameSpace), " ")

	if msg := nameShapeMessage(name); msg != "" {
		return "", NewValidationError(fieldName, msg)
	}

	return name, nil
}

// allowedNameRune reports whether r may appear in a name: a letter or a mark
// that renders as something, or a space, hyphen-minus, apostrophe (straight or
// typographic), or full stop. Ranging over invalid UTF-8 yields U+FFFD, a
// symbol, so it is refused.
func allowedNameRune(r rune) bool {
	switch r {
	case ' ', '-', '\'', '\u2019', '.':
		return true
	}

	return (unicode.IsLetter(r) || unicode.IsMark(r)) &&
		!unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) &&
		!unicode.Is(unicode.Variation_Selector, r)
}

// isNameSpace reports whether r is the one space a name may hold.
func isNameSpace(r rune) bool {
	return r == ' '
}

// nameShapeMessage returns the message of the first shape rule a non-empty
// name breaks, or "" when it keeps them all.
func nameShapeMessage(name string) string {
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)

	switch {
	case !unicode.IsLetter(first) && first != '\'' && first != '\u2019':
		return msgNameStart
	case last == '-':
		return msgNameEnd
	case !strings.ContainsFunc(name, unicode.IsLetter):
		return msgNameLetter
	case utf8.RuneCountInString(name) > MaxPersonNameLength:
		return msgNameTooLong
	}

	return ""
}
