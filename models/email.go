package models

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
	"golang.org/x/text/secure/precis"
)

// Email length limits per RFC 5321 §4.5.3.1.
const (
	maxEmailLength = 254
	maxLocalLength = 64
)

// Repeated email validation messages, hoisted to a single source of truth.
const (
	msgEmailInvalid      = "is not a valid email address"
	msgEmailTooLong      = "exceeds 254 characters"
	msgEmailLocalTooLong = "local part exceeds 64 characters"
	msgEmailBadDomain    = "has an invalid domain"
	msgEmailQuotedLocal  = "must not have a quoted local part"
	msgEmailTrailingDot  = "must not end with a dot"
	msgEmailSingleLabel  = "must have a dot in its domain"
)

// NormalizedEmail is the result of NormalizeEmail. It carries the mailbox
// the address names (Mailbox), the per-row canonical form (Address) used as the
// storage key, and the alias-collapsed Root used to group equivalent mailboxes
// (e.g. Gmail's plus-tags and dot-insensitivity).
type NormalizedEmail struct {
	// Mailbox is the address mail is delivered to, normalized: the
	// PRECIS-folded local part and the lowercased ASCII (IDN Punycode) domain
	// the input named, before provider domain aliases. It differs from Address
	// only for an aliased domain (jane@googlemail.com, whose Address is
	// jane@gmail.com). Send mail, and match the address a person typed, by
	// Mailbox: an aliased domain need not deliver to the same inbox.
	Mailbox string
	// Address is the canonical per-row form: lowercased ASCII domain (IDN
	// Punycode) with provider domain aliases applied, PRECIS-folded local
	// part. Plus tags are preserved here. Quoted local parts keep their
	// surrounding quotes verbatim.
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

// EmailOption narrows what NormalizeEmail accepts. Each option refuses one
// form the broad RFC 5321/5322 set allows; StrictEmail applies them all.
type EmailOption func(*emailRules)

// emailRules is what the EmailOptions of one NormalizeEmail call refuse.
type emailRules struct {
	rejectQuotedLocal   bool
	rejectTrailingDot   bool
	requireDottedDomain bool
}

// RejectQuotedLocal refuses a quoted local part, such as "jane doe"@example.com.
func RejectQuotedLocal() EmailOption {
	return func(r *emailRules) { r.rejectQuotedLocal = true }
}

// RejectTrailingDot refuses an address that ends in the DNS root dot, such as
// jane@example.com., instead of dropping the dot.
func RejectTrailingDot() EmailOption {
	return func(r *emailRules) { r.rejectTrailingDot = true }
}

// RequireDottedDomain refuses a single-label domain, such as jane@localhost.
// An address on the public internet has at least one dot in its domain.
func RequireDottedDomain() EmailOption {
	return func(r *emailRules) { r.requireDottedDomain = true }
}

// StrictEmail applies RejectQuotedLocal, RejectTrailingDot, and
// RequireDottedDomain: the shape of an ordinary address on the public
// internet.
func StrictEmail() EmailOption {
	return func(r *emailRules) {
		r.rejectQuotedLocal = true
		r.rejectTrailingDot = true
		r.requireDottedDomain = true
	}
}

// NormalizeEmail validates an email address and produces its canonical and
// alias-root forms.
//
// Input is flexible. Surrounding whitespace is trimmed and case is folded,
// and the wrappers an address is often pasted in are removed: a mailto:
// scheme, one pair of enclosing angle brackets or quotes (straight or curly),
// and one trailing comma or semicolon. None of those can be part of a valid
// address, so removing them never changes the result for one.
//
// With no options it accepts the broad RFC 5321/5322 punctuation set, quoted
// local parts, a single-label domain, a trailing root dot (dropped), full
// Unicode local parts via PRECIS (RFC 8265), and internationalized domain
// names via IDNA (RFC 5891). Options refuse some of those forms; nil options
// are ignored. A display-name form (Jane <jane@example.com>) and a list of
// addresses are always refused.
//
// Returns a ValidationError (wrapping ErrValidation) on any parse, length,
// PRECIS, or IDNA failure, or a form an option refuses. No error includes the
// input.
func NormalizeEmail(raw string, opts ...EmailOption) (NormalizedEmail, error) {
	rules := emailRulesFrom(opts)

	cleaned := unwrapEmailInput(raw)
	if rules.rejectTrailingDot && strings.HasSuffix(cleaned, ".") {
		return NormalizedEmail{}, NewValidationError("email", msgEmailTrailingDot)
	}

	trimmed, err := prepareEmailInput(cleaned)
	if err != nil {
		return NormalizedEmail{}, err
	}

	normalized, err := normalizeTrimmed(trimmed)
	if err != nil {
		return NormalizedEmail{}, err
	}

	if err = rules.check(normalized); err != nil {
		return NormalizedEmail{}, err
	}

	return normalized, nil
}

// emailRulesFrom applies opts, skipping nil ones. Without options it
// allocates nothing: the rules escape only when an option is called.
func emailRulesFrom(opts []EmailOption) emailRules {
	if len(opts) == 0 {
		return emailRules{}
	}

	rules := new(emailRules)
	for _, opt := range opts {
		if opt != nil {
			opt(rules)
		}
	}

	return *rules
}

// check refuses a normalized address in a form the rules refuse.
func (r emailRules) check(normalized NormalizedEmail) error {
	switch {
	case r.rejectQuotedLocal && normalized.IsQuoted:
		return NewValidationError("email", msgEmailQuotedLocal)
	case r.requireDottedDomain && !strings.Contains(normalized.Domain, "."):
		return NewValidationError("email", msgEmailSingleLabel)
	}

	return nil
}

// normalizeTrimmed normalizes a prepared address, quoted or not.
func normalizeTrimmed(trimmed string) (NormalizedEmail, error) {
	// Quoted local parts are RFC-special: net/mail.ParseAddress unquotes them
	// (turning "john..doe"@example.com into john..doe@example.com), which loses
	// the syntax that made the local part legal. We preserve the original
	// quoted form ourselves while still letting net/mail validate.
	if strings.HasPrefix(trimmed, `"`) {
		return normalizeQuoted(trimmed)
	}

	return normalizeUnquoted(trimmed)
}

// mailtoScheme is the URI scheme of an address copied from a link.
const mailtoScheme = "mailto:"

// unwrapEmailInput trims raw and removes, in order, one trailing comma or
// semicolon, one pair of enclosing wrappers (emailWrapperClosing) whose inside
// holds neither of the pair's runes, and a mailto: scheme in any case,
// trimming after each. None of those can be part of a valid address's outer
// form: angle brackets are refused outright, and a quote that encloses the
// whole input leaves no @domain outside it. Input without them is returned
// trimmed, without allocating.
func unwrapEmailInput(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasSuffix(s, ",") || strings.HasSuffix(s, ";") {
		s = strings.TrimSpace(s[:len(s)-1])
	}

	opening, openSize := utf8.DecodeRuneInString(s)
	last, lastSize := utf8.DecodeLastRuneInString(s)
	if closing, ok := emailWrapperClosing(opening); ok && last == closing && len(s) >= openSize+lastSize {
		inner := s[openSize : len(s)-lastSize]
		if !strings.ContainsRune(inner, opening) && !strings.ContainsRune(inner, closing) {
			s = strings.TrimSpace(inner)
		}
	}

	if len(s) >= len(mailtoScheme) && strings.EqualFold(s[:len(mailtoScheme)], mailtoScheme) {
		s = strings.TrimSpace(s[len(mailtoScheme):])
	}

	return s
}

// emailWrapperClosing returns the rune that closes a pair an address is
// pasted in, given its opening rune: angle brackets, or straight or curly
// quotes.
func emailWrapperClosing(opening rune) (rune, bool) {
	switch opening {
	case '<':
		return '>', true
	case '"', '\'':
		return opening, true
	case '“':
		return '”', true
	case '‘':
		return '’', true
	}

	return 0, false
}

// prepareEmailInput trims whitespace, removes a DNS-style trailing dot, and
// rejects empties, oversize inputs, or display-name (angle-bracket) forms.
func prepareEmailInput(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", NewValidationError("email", msgRequired)
	}
	// A single trailing dot is the DNS root-label form ("example.com."); strip
	// it so net/mail can parse, then carry on as if it had never been there.
	trimmed = strings.TrimSuffix(trimmed, ".")
	if trimmed == "" {
		return "", NewValidationError("email", msgRequired)
	}

	if len(trimmed) > maxEmailLength {
		return "", NewValidationError("email", msgEmailTooLong)
	}

	if strings.ContainsAny(trimmed, "<>") {
		return "", NewValidationError("email", msgEmailInvalid)
	}

	return trimmed, nil
}

// normalizeUnquoted handles addresses with an unquoted local part: net/mail
// validates RFC syntax, PRECIS folds Unicode, and provider rules apply.
func normalizeUnquoted(trimmed string) (NormalizedEmail, error) {
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Name != "" {
		return NormalizedEmail{}, NewValidationError("email", msgEmailInvalid)
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
		return NormalizedEmail{}, NewValidationError("email", msgEmailInvalid)
	}

	asciiDomain, err := canonicalDomain(domain)
	if err != nil {
		return NormalizedEmail{}, NewValidationError("email", msgEmailBadDomain)
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
		return NormalizedEmail{}, NewValidationError("email", msgEmailLocalTooLong)
	}

	canonicalAddress := foldedLocal + "@" + canonical
	if len(canonicalAddress) > maxEmailLength {
		return NormalizedEmail{}, NewValidationError("email", msgEmailTooLong)
	}

	// Folding a non-ASCII local part normalizes it, which can turn a rune into
	// a special one (U+037E GREEK QUESTION MARK becomes ';'), so the folded
	// address must still parse as itself. An ASCII local part is only
	// lower-cased, which keeps it valid.
	if !isASCII(local) && !parsesAsItself(canonicalAddress) {
		return NormalizedEmail{}, NewValidationError("email", "has an invalid local part")
	}

	// The mailbox is the address unless a provider alias renamed the domain.
	mailbox := canonicalAddress
	if asciiDomain != canonical {
		if mailbox = foldedLocal + "@" + asciiDomain; len(mailbox) > maxEmailLength {
			return NormalizedEmail{}, NewValidationError("email", msgEmailTooLong)
		}
	}

	return NormalizedEmail{
		Mailbox:      mailbox,
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
		return NormalizedEmail{}, NewValidationError("email", msgEmailInvalid)
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
		return NormalizedEmail{}, NewValidationError("email", msgEmailInvalid)
	}

	quotedLocal := trimmed[:closeIdx+1]
	rawDomain := trimmed[closeIdx+2:]

	if quotedLocal == `""` {
		return NormalizedEmail{}, NewValidationError("email", "has an empty local part")
	}

	if len(quotedLocal) > maxLocalLength {
		return NormalizedEmail{}, NewValidationError("email", msgEmailLocalTooLong)
	}

	canonical, err := canonicalDomain(rawDomain)
	if err != nil {
		return NormalizedEmail{}, NewValidationError("email", msgEmailBadDomain)
	}

	address := quotedLocal + "@" + canonical
	if len(address) > maxEmailLength {
		return NormalizedEmail{}, NewValidationError("email", msgEmailTooLong)
	}

	return NormalizedEmail{
		Mailbox:      address,
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

	// IDNA maps some runes away (a soft hyphen, for one), so a domain of only
	// those is empty once mapped.
	if ascii == "" {
		return "", NewValidationError("email", "has an empty domain")
	}

	return ascii, nil
}

// parsesAsItself reports whether address parses as a bare address equal to
// itself.
func parsesAsItself(address string) bool {
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Name == "" && parsed.Address == address
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
