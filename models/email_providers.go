package models

// ProviderRule describes how a mailbox provider (or class of providers) collapses
// distinct addresses to the same underlying mailbox. The default rule treats "+"
// as a tag separator and preserves dots; provider-specific rules override.
//
// Caveats:
//   - Outlook's "+" aliasing is config-dependent at the upstream MTA; treating
//     "+" as an alias separator here may collapse rows the provider actually
//     delivers separately. We accept that risk since the result is only used as
//     a queryable alias key — the original address is preserved as the unique
//     row identity.
//   - Yahoo historically used "-" as a Disposable Address Plus separator, but
//     that conflicts with legitimate hyphenated local parts (e.g. "mary-anne"
//     would collapse to "mary"). We only honor "+" for Yahoo.
//   - FastMail's subdomain alias trick (user@alias.fastmail.com) is not handled;
//     only "+" tagging is collapsed.
type ProviderRule struct {
	// Separator is the rune that begins a tag suffix on the local part. Everything
	// from the first occurrence onward is stripped when computing the alias root.
	Separator rune
	// StripDots removes every "." from the local part when computing the alias
	// root. Only Gmail/Googlemail behaves this way.
	StripDots bool
}

// domainAliases collapses provider domain variants to one canonical domain so
// that aliased mailboxes (e.g. googlemail.com vs gmail.com) share a root.
var domainAliases = map[string]string{ //nolint:gochecknoglobals // provider rule table
	"googlemail.com": "gmail.com",
	"ymail.com":      "yahoo.com",
	"rocketmail.com": "yahoo.com",
	"me.com":         "icloud.com",
	"mac.com":        "icloud.com",
}

// ProviderRules holds the per-provider alias collapse policy, keyed by the
// canonical domain (post domainAliases lookup).
var ProviderRules = map[string]ProviderRule{ //nolint:gochecknoglobals // provider rule table
	"gmail.com":      {Separator: '+', StripDots: true},
	"outlook.com":    {Separator: '+'},
	"hotmail.com":    {Separator: '+'},
	"hotmail.co.uk":  {Separator: '+'},
	"hotmail.fr":     {Separator: '+'},
	"hotmail.de":     {Separator: '+'},
	"live.com":       {Separator: '+'},
	"msn.com":        {Separator: '+'},
	"yahoo.com":      {Separator: '+'},
	"yahoo.co.uk":    {Separator: '+'},
	"yahoo.fr":       {Separator: '+'},
	"yahoo.de":       {Separator: '+'},
	"fastmail.com":   {Separator: '+'},
	"fastmail.fm":    {Separator: '+'},
	"icloud.com":     {Separator: '+'},
	"proton.me":      {Separator: '+'},
	"protonmail.com": {Separator: '+'},
	"protonmail.ch":  {Separator: '+'},
	"pm.me":          {Separator: '+'},
}

// defaultProviderRule is applied when the domain has no entry in ProviderRules.
// We conservatively strip "+" tags so the alias root for an unknown provider
// still collapses obvious plus-addressed forms.
var defaultProviderRule = ProviderRule{Separator: '+'} //nolint:gochecknoglobals // provider rule table

// LookupProviderRule resolves a lowercased domain to its canonical form and the
// provider rule that governs its alias-root computation. Unknown domains return
// (domain, defaultProviderRule).
func LookupProviderRule(domain string) (canonicalDomain string, rule ProviderRule) {
	canonicalDomain = domain
	if alias, ok := domainAliases[domain]; ok {
		canonicalDomain = alias
	}

	if r, ok := ProviderRules[canonicalDomain]; ok {
		return canonicalDomain, r
	}

	return canonicalDomain, defaultProviderRule
}
