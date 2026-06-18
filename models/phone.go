package models

import (
	"regexp"
	"strings"
)

// phoneDigits strips every non-digit character from a phone string.
var phoneDigits = regexp.MustCompile(`\D`)

// e164Pattern matches the canonical E.164 form: '+' followed by 1–15 digits,
// where the first digit is non-zero.
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// NormalizePhone canonicalizes a phone string to E.164. A bare 10-digit number
// is treated as North American. An input that begins with '+' is preserved
// verbatim apart from non-digit stripping; anything else is prefixed with '+'.
//
// Returns a ValidationError (wrapping ErrValidation) on empty input or on a
// result that fails the E.164 regex. The errorField argument is the field name
// reported in the validation error (e.g. "phone" or "e164") so the caller can
// shape the error to match its own DTO.
func NormalizePhone(raw, errorField string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", NewValidationError(errorField, "is required")
	}

	digits := phoneDigits.ReplaceAllString(trimmed, "")

	var candidate string

	switch {
	case strings.HasPrefix(trimmed, "+"):
		candidate = "+" + digits
	case len(digits) == 10:
		candidate = "+1" + digits
	default:
		candidate = "+" + digits
	}

	if !e164Pattern.MatchString(candidate) {
		return "", NewValidationError(errorField, "is not a valid phone number")
	}

	return candidate, nil
}
