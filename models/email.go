package models

import (
	"net/mail"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/text/secure/precis"
)

// Email length limits per RFC 5321 §4.5.3.1.
const (
	maxEmailLength = 254
	maxLocalLength = 64
)

// NormalizedEmail is the result of NormalizeEmail. It carries both the
// per-row canonical form (Address) used as the storage key and the
// alias-collapsed Root used to group equivalent mailboxes (e.g. Gmail's
// plus-tags and dot-insensitivity).
type NormalizedEmail struct {
	// Address is the canonical per-row form: lowercased ASCII domain (IDN
	// Punycode), PRECIS-folded local part. Plus tags are preserved here.
	// Quoted local parts keep their surrounding quotes verbatim.
	Address string
	// Root is the alias-collapsed form: provider rules applied (e.g. plus
	// tag stripped, Gmail dots removed). May equal Address when no rules apply.
	// For quoted local parts, Root equals Address (provider rules are skipped).
	Root string
	// Domain is the ASCII (Punycode) canonical domain, post-aliasing
	// (e.g. googlemail.com → gmail.com).
	Domain string
	// DisplayLocal is the local part as it appeared in the input
	// (post-whitespace-trim, pre-folding). Useful for display.
	DisplayLocal string
	// IsQuoted is true when the local part was a quoted string ("...@...").
	// Quoted local parts are opaque per RFC 5321 §4.1.2 and skip provider rules.
	IsQuoted bool
}

// NormalizeEmail validates an email address and produces its canonical and
// alias-root forms. It accepts the broad RFC 5321/5322 punctuation set,
// quoted local parts, full Unicode local parts via PRECIS (RFC 8265), and
// internationalized domain names via IDNA (RFC 5891).
//
// Returns a ValidationError (wrapping ErrValidation) on any parse, length,
// PRECIS, or IDNA failure.
func NormalizeEmail(raw string) (NormalizedEmail, error) {
	trimmed, err := prepareEmailInput(raw)
	if err != nil {
		return NormalizedEmail{}, err
	}
	// Quoted local parts are RFC-special: net/mail.ParseAddress unquotes them
	// (turning "john..doe"@example.com into john..doe@example.com), which loses
	// the syntax that made the local part legal. We preserve the original
	// quoted form ourselves while still letting net/mail validate.
	if strings.HasPrefix(trimmed, `"`) {
		return normalizeQuoted(trimmed)
	}

	return normalizeUnquoted(trimmed)
}

// prepareEmailInput trims whitespace, removes a DNS-style trailing dot, and
// rejects empties, oversize inputs, or display-name (angle-bracket) forms.
func prepareEmailInput(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", NewValidationError("email", "is required")
	}
	// A single trailing dot is the DNS root-label form ("example.com."); strip
	// it so net/mail can parse, then carry on as if it had never been there.
	trimmed = strings.TrimSuffix(trimmed, ".")
	if trimmed == "" {
		return "", NewValidationError("email", "is required")
	}

	if len(trimmed) > maxEmailLength {
		return "", NewValidationError("email", "exceeds 254 characters")
	}

	if strings.ContainsAny(trimmed, "<>") {
		return "", NewValidationError("email", "is not a valid email address")
	}

	return trimmed, nil
}

// normalizeUnquoted handles addresses with an unquoted local part: net/mail
// validates RFC syntax, PRECIS folds Unicode, and provider rules apply.
func normalizeUnquoted(trimmed string) (NormalizedEmail, error) {
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Name != "" {
		return NormalizedEmail{}, NewValidationError("email", "is not a valid email address")
	}

	return buildUnquoted(parsed.Address)
}

// buildUnquoted assembles the canonical and alias-root forms from an
// already-parsed, RFC-valid unquoted address ("local@domain"). It is split out
// from normalizeUnquoted so its defensive guards (split failure, invalid
// domain/local, oversize assembly) can be exercised directly with crafted
// inputs that the net/mail parse gate would otherwise reject.
func buildUnquoted(address string) (NormalizedEmail, error) {
	local, domain, ok := splitLocalDomain(address)
	if !ok {
		return NormalizedEmail{}, NewValidationError("email", "is not a valid email address")
	}

	asciiDomain, err := canonicalDomain(domain)
	if err != nil {
		return NormalizedEmail{}, NewValidationError("email", "has an invalid domain")
	}

	canonical, rule := LookupProviderRule(asciiDomain)

	// splitLocalDomain guarantees a non-empty local, and foldLocal never returns
	// an empty result without also returning an error (PRECIS rejects runes that
	// would otherwise map away), so an empty foldedLocal here implies a fold
	// error, which is reported as an invalid local part.
	foldedLocal, err := foldLocal(local)
	if err != nil {
		return NormalizedEmail{}, NewValidationError("email", "has an invalid local part")
	}

	if len(foldedLocal) > maxLocalLength {
		return NormalizedEmail{}, NewValidationError("email", "local part exceeds 64 characters")
	}

	canonicalAddress := foldedLocal + "@" + canonical
	if len(canonicalAddress) > maxEmailLength {
		return NormalizedEmail{}, NewValidationError("email", "exceeds 254 characters")
	}

	return NormalizedEmail{
		Address:      canonicalAddress,
		Root:         aliasRoot(foldedLocal, canonical, rule),
		Domain:       canonical,
		DisplayLocal: local,
		IsQuoted:     false,
	}, nil
}

// normalizeQuoted handles the "..."@domain form. The original quoted local is
// preserved verbatim; only the domain is canonicalized. Provider alias rules
// are skipped because quoted strings are opaque per RFC 5321 §4.1.2.
func normalizeQuoted(trimmed string) (NormalizedEmail, error) {
	if _, err := mail.ParseAddress(trimmed); err != nil {
		return NormalizedEmail{}, NewValidationError("email", "is not a valid email address")
	}

	return buildQuoted(trimmed)
}

// buildQuoted assembles the canonical form from an already-parsed, RFC-valid
// quoted address (`"local"@domain`). It is split out from normalizeQuoted so
// its defensive guards (bad closing-quote position, empty quoted local,
// oversize assembly) can be exercised directly with crafted inputs that the
// net/mail parse gate would otherwise reject.
func buildQuoted(trimmed string) (NormalizedEmail, error) {
	closeIdx := closingQuoteIndex(trimmed)
	if closeIdx < 0 || closeIdx+1 >= len(trimmed) || trimmed[closeIdx+1] != '@' {
		return NormalizedEmail{}, NewValidationError("email", "is not a valid email address")
	}

	quotedLocal := trimmed[:closeIdx+1]
	rawDomain := trimmed[closeIdx+2:]

	if quotedLocal == `""` {
		return NormalizedEmail{}, NewValidationError("email", "has an empty local part")
	}

	if len(quotedLocal) > maxLocalLength {
		return NormalizedEmail{}, NewValidationError("email", "local part exceeds 64 characters")
	}

	canonical, err := canonicalDomain(rawDomain)
	if err != nil {
		return NormalizedEmail{}, NewValidationError("email", "has an invalid domain")
	}

	address := quotedLocal + "@" + canonical
	if len(address) > maxEmailLength {
		return NormalizedEmail{}, NewValidationError("email", "exceeds 254 characters")
	}

	return NormalizedEmail{
		Address:      address,
		Root:         address,
		Domain:       canonical,
		DisplayLocal: quotedLocal,
		IsQuoted:     true,
	}, nil
}

// closingQuoteIndex returns the byte index of the closing '"' that pairs with
// the leading '"' at position 0, honoring backslash escapes. Returns -1 if the
// quoted string is not properly terminated.
func closingQuoteIndex(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}

	return -1
}

// splitLocalDomain splits an address at the last unescaped "@", returning the
// local part, the domain, and ok=false if either side is empty.
func splitLocalDomain(addr string) (local, domain string, ok bool) {
	at := strings.LastIndex(addr, "@")
	if at <= 0 || at >= len(addr)-1 {
		return "", "", false
	}

	return addr[:at], addr[at+1:], true
}

// canonicalDomain lowercases, strips any trailing dot, and Punycode-encodes a
// domain via IDNA Lookup profile. Returns an error on invalid IDN input.
func canonicalDomain(domain string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(domain))

	d = strings.TrimSuffix(d, ".")
	if d == "" {
		return "", NewValidationError("email", "has an empty domain")
	}

	ascii, err := idna.Lookup.ToASCII(d)
	if err != nil {
		return "", err
	}

	return ascii, nil
}

// foldLocal applies PRECIS UsernameCaseMapped to the local part — case-folds
// Unicode, rejects bidi/control mischief, and produces a stable canonical form.
// Falls back to a plain ASCII-lowercase when the local part is pure ASCII so
// punctuation-heavy RFC 5321 forms (e.g. !#$%&'*+-/=?^_`{|}~) pass through.
func foldLocal(local string) (string, error) {
	if isASCII(local) {
		return strings.ToLower(local), nil
	}

	return precis.UsernameCaseMapped.String(local)
}

// isASCII reports whether s contains only 7-bit ASCII bytes.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] > 127 {
			return false
		}
	}

	return true
}

// aliasRoot computes the provider-collapsed root form of an address. The
// separator-suffix is stripped from the local part, and (if the provider rule
// says so) all dots are removed before the local is rejoined to the canonical
// domain.
func aliasRoot(local, domain string, rule ProviderRule) string {
	rootLocal := local
	if rule.Separator != 0 {
		if i := strings.IndexRune(rootLocal, rule.Separator); i >= 0 {
			rootLocal = rootLocal[:i]
		}
	}

	if rule.StripDots {
		rootLocal = strings.ReplaceAll(rootLocal, ".", "")
	}

	if rootLocal == "" {
		// Defensive: never return a bare "@domain" — fall back to the full local.
		rootLocal = local
	}

	return rootLocal + "@" + domain
}
