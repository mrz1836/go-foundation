package models

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/mrz1836/go-sanitize"
	"github.com/nyaruka/phonenumbers"
)

// ErrUnknownPhoneRegion is returned by ParsePhone when WithDefaultRegion names
// a region the numbering-plan metadata doesn't know. It is a caller's
// configuration error, not a ValidationError.
var ErrUnknownPhoneRegion = errors.New("models: unknown phone region")

// maxPhoneInputLength bounds the input ParsePhone examines. A phone number
// with its punctuation, a label, or a URI scheme is far shorter.
const maxPhoneInputLength = 64

// maxE164Length is the longest E.164 number: + and 15 digits (ITU-T E.164 §6).
const maxE164Length = 16

// The fixed messages of ParsePhone's validation errors. None includes the
// input.
const (
	msgPhoneInvalid     = "is not a valid phone number"
	msgPhoneTooLong     = "is too long to be a phone number"
	msgPhoneCountryCode = "must start with + and its country code"
	msgPhoneExtension   = "must not have an extension"
	msgPhoneUnassigned  = "is not a number in use in its region"
)

// PhoneLineType is the kind of line a numbering plan assigns a phone number
// to.
type PhoneLineType string

// Recognized PhoneLineType values.
const (
	PhoneLineTypeMobile            PhoneLineType = "mobile"
	PhoneLineTypeFixedLine         PhoneLineType = "fixed_line"
	PhoneLineTypeFixedLineOrMobile PhoneLineType = "fixed_line_or_mobile"
	PhoneLineTypeTollFree          PhoneLineType = "toll_free"
	PhoneLineTypePremiumRate       PhoneLineType = "premium_rate"
	PhoneLineTypeSharedCost        PhoneLineType = "shared_cost"
	PhoneLineTypeVOIP              PhoneLineType = "voip"
	PhoneLineTypePersonalNumber    PhoneLineType = "personal_number"
	PhoneLineTypePager             PhoneLineType = "pager"
	PhoneLineTypeUAN               PhoneLineType = "uan"
	PhoneLineTypeVoicemail         PhoneLineType = "voicemail"
	PhoneLineTypeUnknown           PhoneLineType = "unknown"
)

// lineTypes maps libphonenumber's number types to PhoneLineType.
//
//nolint:gochecknoglobals // read-only lookup table
var lineTypes = map[phonenumbers.PhoneNumberType]PhoneLineType{
	phonenumbers.MOBILE:               PhoneLineTypeMobile,
	phonenumbers.FIXED_LINE:           PhoneLineTypeFixedLine,
	phonenumbers.FIXED_LINE_OR_MOBILE: PhoneLineTypeFixedLineOrMobile,
	phonenumbers.TOLL_FREE:            PhoneLineTypeTollFree,
	phonenumbers.PREMIUM_RATE:         PhoneLineTypePremiumRate,
	phonenumbers.SHARED_COST:          PhoneLineTypeSharedCost,
	phonenumbers.VOIP:                 PhoneLineTypeVOIP,
	phonenumbers.PERSONAL_NUMBER:      PhoneLineTypePersonalNumber,
	phonenumbers.PAGER:                PhoneLineTypePager,
	phonenumbers.UAN:                  PhoneLineTypeUAN,
	phonenumbers.VOICEMAIL:            PhoneLineTypeVoicemail,
}

// ParsedPhone is a phone number ParsePhone accepted, in standard forms.
type ParsedPhone struct {
	// E164 is the canonical form to store and compare, such as +13055550100.
	E164 string

	// International is the standard display form, such as +1 305-555-0100.
	International string

	// RegionCode is the ISO 3166-1 alpha-2 code of the number's region, such
	// as "US"; "001" for a non-geographic number; or empty when the numbering
	// plan doesn't place it in one region.
	RegionCode string

	// CountryCallingCode is the number's country calling code, such as 1.
	CountryCallingCode int

	// NationalNumber is the number without its country calling code, as
	// digits.
	NationalNumber string

	// LineType is the kind of line the numbering plan assigns the number to,
	// or PhoneLineTypeUnknown.
	LineType PhoneLineType
}

// PhoneOption adjusts what ParsePhone accepts.
type PhoneOption func(*phoneRules)

// phoneRules is what the PhoneOptions of one ParsePhone call set.
type phoneRules struct {
	defaultRegion string
	requireValid  bool
	keypadLetters bool
}

// WithDefaultRegion sets the region, an ISO 3166-1 alpha-2 code such as "US",
// that a number written without its country code belongs to. It also sets the
// international dialing prefix ParsePhone recognizes (011 for "US", 00 for
// "GB"). Without it, a number must start with + and its country code.
func WithDefaultRegion(region string) PhoneOption {
	return func(r *phoneRules) { r.defaultRegion = strings.ToUpper(strings.TrimSpace(region)) }
}

// RequireValidNumber refuses a number outside the ranges its region assigns,
// such as 000-000-0000 or 123-456-7890. Without it, ParsePhone checks only that
// the number has a length its region allows, which accepts numbers that are
// not in use but survives numbering-plan changes the metadata doesn't know yet.
func RequireValidNumber() PhoneOption {
	return func(r *phoneRules) { r.requireValid = true }
}

// KeypadLetters reads letters as the keypad digits they stand for, as in
// 1-800-FLOWERS. Without it, letters are dropped with the rest of what isn't a
// digit, so a lettered number is too short and refused.
func KeypadLetters() PhoneOption {
	return func(r *phoneRules) { r.keypadLetters = true }
}

// ParsePhone validates a phone number against its region's numbering plan
// (libphonenumber's metadata) and returns it in standard forms.
//
// Input is flexible. Before parsing, everything but the digits and the plus
// signs is dropped (go-sanitize's PhoneNumber): spaces, dashes of any kind,
// dots, slashes, parentheses, labels such as "Phone:", and URI schemes such as
// tel:, sms:, and callto:. Digits in any script count, such as full-width or
// Arabic-Indic ones. An international dialing prefix (00, or 011 in "US")
// stands for the plus sign when WithDefaultRegion is set, and the trunk 0 a
// number written with its country code may carry, as in +44 (0)20, is dropped.
//
// The result must be one number of a length its region allows and at most
// E.164's 15 digits. An extension is refused, because E.164 can't hold one; it
// is found in the input as written, before its digits could merge into the
// number's. Options set a default region, require a number in use, or read
// keypad letters. Nil options are ignored.
//
// Returns a ValidationError (wrapping ErrValidation) on input that is not
// such a number, and no error includes the input. A default region the
// metadata doesn't know is ErrUnknownPhoneRegion.
func ParsePhone(raw string, opts ...PhoneOption) (ParsedPhone, error) {
	rules := phoneRulesFrom(opts)

	region, err := rules.region()
	if err != nil {
		return ParsedPhone{}, err
	}

	candidate, err := phoneCandidate(raw, region, rules.keypadLetters)
	if err != nil {
		return ParsedPhone{}, err
	}

	number, err := phonenumbers.Parse(candidate, region)
	if err != nil {
		if region == phonenumbers.UNKNOWN_REGION && !strings.Contains(candidate, "+") {
			return ParsedPhone{}, NewValidationError("phone", msgPhoneCountryCode)
		}

		return ParsedPhone{}, NewValidationError("phone", msgPhoneInvalid)
	}

	if err = checkPhoneNumber(number, rules.requireValid); err != nil {
		return ParsedPhone{}, err
	}

	parsed := parsedPhone(number)
	if !isStableE164(parsed.E164) {
		return ParsedPhone{}, NewValidationError("phone", msgPhoneInvalid)
	}

	return parsed, nil
}

// isStableE164 reports whether e164 parses back to itself. A number dialed
// through an international prefix can keep its region's trunk prefix in the
// national number, which parsing the E.164 form then strips: such a form names
// a different number, so it is no standard form.
func isStableE164(e164 string) bool {
	number, err := phonenumbers.Parse(e164, phonenumbers.UNKNOWN_REGION)
	return err == nil && phonenumbers.Format(number, phonenumbers.E164) == e164
}

// phoneRulesFrom applies opts, skipping nil ones. Without options it
// allocates nothing: the rules escape only when an option is called.
func phoneRulesFrom(opts []PhoneOption) phoneRules {
	if len(opts) == 0 {
		return phoneRules{}
	}

	rules := new(phoneRules)
	for _, opt := range opts {
		if opt != nil {
			opt(rules)
		}
	}

	return *rules
}

// region returns the region libphonenumber parses national numbers in:
// UNKNOWN_REGION without a default region, which demands a country code.
func (r phoneRules) region() (string, error) {
	if r.defaultRegion == "" {
		return phonenumbers.UNKNOWN_REGION, nil
	}
	if phonenumbers.GetCountryCodeForRegion(r.defaultRegion) == 0 {
		return "", fmt.Errorf("%w: %q", ErrUnknownPhoneRegion, r.defaultRegion)
	}

	return r.defaultRegion, nil
}

// phoneCandidate trims raw, refuses it when it is empty, oversized, or carries
// an extension, and returns what libphonenumber parses: the digits and plus
// signs alone, unless keypadLetters keeps the letters for it to read.
func phoneCandidate(raw, region string, keypadLetters bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return "", NewValidationError("phone", msgRequired)
	case len(trimmed) > maxPhoneInputLength:
		return "", NewValidationError("phone", msgPhoneTooLong)
	case hasPhoneExtension(trimmed, region):
		return "", NewValidationError("phone", msgPhoneExtension)
	case keypadLetters:
		return trimmed, nil
	}

	candidate := sanitize.PhoneNumber(trimmed)
	if candidate == "" {
		return "", NewValidationError("phone", msgPhoneInvalid)
	}

	return candidate, nil
}

// hasPhoneExtension reports whether input, as written, carries an extension
// (x12, ext. 12, #12, ;ext=12, ,,12, and the like) that libphonenumber
// recognizes. Every extension marker is a letter or one of extensionMarks, so
// input with neither is not parsed for one.
func hasPhoneExtension(input, region string) bool {
	if !strings.ContainsFunc(input, unicode.IsLetter) && !strings.ContainsAny(input, extensionMarks) {
		return false
	}

	number, err := phonenumbers.Parse(input, region)
	return err == nil && number.GetExtension() != ""
}

// extensionMarks are the characters other than letters that libphonenumber
// reads as the start of an extension.
const extensionMarks = "#＃~～,;"

// checkPhoneNumber refuses a parsed number with an extension; one of a length
// its region doesn't allow, or allows only for local dialing (a number without
// its area code, which no E.164 form names); one longer than E.164's 15 digits
// (some regions' metadata allows longer national numbers); and, when
// requireValid is set, one outside the ranges its region assigns.
func checkPhoneNumber(number *phonenumbers.PhoneNumber, requireValid bool) error {
	switch {
	case number.GetExtension() != "":
		return NewValidationError("phone", msgPhoneExtension)
	case phonenumbers.IsPossibleNumberWithReason(number) != phonenumbers.IS_POSSIBLE,
		len(phonenumbers.Format(number, phonenumbers.E164)) > maxE164Length:
		return NewValidationError("phone", msgPhoneInvalid)
	case requireValid && !phonenumbers.IsValidNumber(number):
		return NewValidationError("phone", msgPhoneUnassigned)
	}

	return nil
}

// parsedPhone describes an accepted number in its standard forms.
func parsedPhone(number *phonenumbers.PhoneNumber) ParsedPhone {
	lineType, ok := lineTypes[phonenumbers.GetNumberType(number)]
	if !ok {
		lineType = PhoneLineTypeUnknown
	}

	return ParsedPhone{
		E164:               phonenumbers.Format(number, phonenumbers.E164),
		International:      phonenumbers.Format(number, phonenumbers.INTERNATIONAL),
		RegionCode:         phonenumbers.GetRegionCodeForNumber(number),
		CountryCallingCode: int(number.GetCountryCode()),
		NationalNumber:     phonenumbers.GetNationalSignificantNumber(number),
		LineType:           lineType,
	}
}
