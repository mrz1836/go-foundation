package models_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

// TestNormalizeEmail covers RFC 5321/5322/6531 syntax (broad punctuation,
// quoted local parts, Unicode and IDN), provider-specific alias rules
// (Gmail/Googlemail dot+plus, Outlook/Yahoo/iCloud/FastMail/Proton plus),
// and length / structural validation. Each case asserts the canonical
// per-row Address, the alias-collapsed Root, the canonical Domain, and
// whether an error was expected.
func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	const (
		oneA64    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		oneA65    = oneA64 + "a"
		broadPunc = `a!#$%&'*+-/=?^_` + "`" + `{|}~`
	)
	// A 254-char address: 64-char local + "@" + 189-char domain. Each domain
	// label respects the 63-char DNS label limit. Total: 64 + 1 + 189 = 254.
	addr254Local := strings.Repeat("a", 64)
	addr254Domain := strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + "." + strings.Repeat("a", 58) + ".co"
	addr254 := addr254Local + "@" + addr254Domain
	// A 255-char total: same layout but one more byte in the trailing label.
	addr255Domain := strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + "." + strings.Repeat("a", 59) + ".co"
	addr255 := addr254Local + "@" + addr255Domain

	cases := []struct {
		name       string
		input      string
		wantAddr   string
		wantRoot   string
		wantDomain string
		wantQuoted bool
		wantErr    bool
	}{
		// ── A. basic RFC valid ───────────────────────────────────────────
		{name: "simple lowercase", input: "a@b.co", wantAddr: "a@b.co", wantRoot: "a@b.co", wantDomain: "b.co"},
		{name: "trims whitespace and lowercases", input: "  Jane@Example.com  ", wantAddr: "jane@example.com", wantRoot: "jane@example.com", wantDomain: "example.com"}, //nolint:goconst // "example.com" is intentional fixture data
		{name: "dot in local kept for unknown provider", input: "user.name@example.com", wantAddr: "user.name@example.com", wantRoot: "user.name@example.com", wantDomain: "example.com"},
		{name: "underscore in local", input: "user_name@example.com", wantAddr: "user_name@example.com", wantRoot: "user_name@example.com", wantDomain: "example.com"},
		{name: "all-digit local", input: "1234567890@example.com", wantAddr: "1234567890@example.com", wantRoot: "1234567890@example.com", wantDomain: "example.com"},
		{name: "broad RFC 5321 punctuation", input: broadPunc + "@example.com", wantAddr: strings.ToLower(broadPunc) + "@example.com", wantRoot: "a!#$%&'*@example.com", wantDomain: "example.com"},
		{name: "hyphenated local kept", input: "first-last@example.com", wantAddr: "first-last@example.com", wantRoot: "first-last@example.com", wantDomain: "example.com"},
		{name: "single-char domain TLD", input: "user@a.b", wantAddr: "user@a.b", wantRoot: "user@a.b", wantDomain: "a.b"},

		// ── B. quoted local parts (RFC 5321 §4.1.2 — opaque, no alias rules) ─
		{name: "quoted consecutive dots", input: `"john..doe"@example.com`, wantAddr: `"john..doe"@example.com`, wantRoot: `"john..doe"@example.com`, wantDomain: "example.com", wantQuoted: true},
		{name: "quoted with space", input: `"john doe"@example.com`, wantAddr: `"john doe"@example.com`, wantRoot: `"john doe"@example.com`, wantDomain: "example.com", wantQuoted: true},
		{name: "quoted at-sign in local", input: `"@"@example.com`, wantAddr: `"@"@example.com`, wantRoot: `"@"@example.com`, wantDomain: "example.com", wantQuoted: true},
		{name: "quoted empty rejected", input: `""@example.com`, wantErr: true},

		// ── C. Unicode / IDN (RFC 6531 + RFC 5891) ───────────────────────
		{name: "Chinese local with IDN domain", input: "用户@例え.jp", wantAddr: "用户@xn--r8jz45g.jp", wantRoot: "用户@xn--r8jz45g.jp", wantDomain: "xn--r8jz45g.jp"}, //nolint:gosmopolitan // intentional Unicode fixture to exercise IDN + PRECIS
		{name: "German umlaut local", input: "Bücher@example.com", wantAddr: "bücher@example.com", wantRoot: "bücher@example.com", wantDomain: "example.com"},
		{name: "IDN domain only", input: "user@münchen.de", wantAddr: "user@xn--mnchen-3ya.de", wantRoot: "user@xn--mnchen-3ya.de", wantDomain: "xn--mnchen-3ya.de"},
		{name: "Spanish IDN domain", input: "user@españa.com", wantAddr: "user@xn--espaa-rta.com", wantRoot: "user@xn--espaa-rta.com", wantDomain: "xn--espaa-rta.com"},
		{name: "Latin diacritic case-folded", input: "José@example.com", wantAddr: "josé@example.com", wantRoot: "josé@example.com", wantDomain: "example.com"},
		{name: "emoji local rejected by PRECIS", input: "🙂@example.com", wantErr: true},

		// ── D. invalid RFC ───────────────────────────────────────────────
		{name: "empty input", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
		{name: "no at sign", input: "noatsign", wantErr: true},
		{name: "empty local", input: "@nodomain.com", wantErr: true},
		{name: "empty domain", input: "local@", wantErr: true},
		{name: "two at signs", input: "a@b@c.com", wantErr: true},
		{name: "leading dot rejected", input: ".leading@example.com", wantErr: true},
		{name: "trailing dot in local rejected", input: "trailing.@example.com", wantErr: true},
		{name: "consecutive dots unquoted rejected", input: "double..dot@example.com", wantErr: true},
		{name: "display-name form rejected", input: "Jane <jane@example.com>", wantErr: true},
		{name: "bare angle brackets rejected", input: "<jane@example.com>", wantErr: true},
		{name: "local too long", input: oneA65 + "@example.com", wantErr: true},
		{name: "total too long", input: addr255, wantErr: true},

		// ── E. Gmail rules (alias domain + dots + plus) ──────────────────
		{name: "gmail no tag self root", input: "jane@gmail.com", wantAddr: "jane@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},
		{name: "gmail plus tag stripped", input: "jane+tag@gmail.com", wantAddr: "jane+tag@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},
		{name: "gmail dots stripped from root", input: "j.a.n.e@gmail.com", wantAddr: "j.a.n.e@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},
		{name: "googlemail collapses to gmail", input: "JANE@GoogleMail.com", wantAddr: "jane@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},
		{name: "gmail first plus wins", input: "jane+a+b@gmail.com", wantAddr: "jane+a+b@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},
		{name: "gmail dots and plus combined", input: "ja.ne+tag@googlemail.com", wantAddr: "ja.ne+tag@gmail.com", wantRoot: "jane@gmail.com", wantDomain: "gmail.com"},

		// ── F. Outlook / Hotmail / Live / MSN (plus only, dots preserved) ─
		{name: "outlook dots preserved", input: "Jane.Doe@outlook.com", wantAddr: "jane.doe@outlook.com", wantRoot: "jane.doe@outlook.com", wantDomain: "outlook.com"},
		{name: "hotmail plus tag stripped", input: "jane+spam@hotmail.com", wantAddr: "jane+spam@hotmail.com", wantRoot: "jane@hotmail.com", wantDomain: "hotmail.com"},
		{name: "live.com plus", input: "jane+x@live.com", wantAddr: "jane+x@live.com", wantRoot: "jane@live.com", wantDomain: "live.com"},
		{name: "msn.com self root", input: "jane@msn.com", wantAddr: "jane@msn.com", wantRoot: "jane@msn.com", wantDomain: "msn.com"},

		// ── G. Yahoo (plus only — dash is NOT a separator) ───────────────
		{name: "yahoo dash kept in root", input: "jane-disposable@yahoo.com", wantAddr: "jane-disposable@yahoo.com", wantRoot: "jane-disposable@yahoo.com", wantDomain: "yahoo.com"},
		{name: "yahoo plus stripped", input: "jane+tag@yahoo.com", wantAddr: "jane+tag@yahoo.com", wantRoot: "jane@yahoo.com", wantDomain: "yahoo.com"},
		{name: "ymail collapses to yahoo", input: "jane.doe@ymail.com", wantAddr: "jane.doe@yahoo.com", wantRoot: "jane.doe@yahoo.com", wantDomain: "yahoo.com"},

		// ── H. iCloud / Me / Mac → icloud.com ────────────────────────────
		{name: "icloud plus stripped", input: "jane+x@icloud.com", wantAddr: "jane+x@icloud.com", wantRoot: "jane@icloud.com", wantDomain: "icloud.com"},
		{name: "me.com aliased to icloud", input: "jane@me.com", wantAddr: "jane@icloud.com", wantRoot: "jane@icloud.com", wantDomain: "icloud.com"},
		{name: "mac.com aliased to icloud", input: "jane@mac.com", wantAddr: "jane@icloud.com", wantRoot: "jane@icloud.com", wantDomain: "icloud.com"},

		// ── I. FastMail (plus, no subdomain trick) ───────────────────────
		{name: "fastmail plus stripped", input: "jane+x@fastmail.com", wantAddr: "jane+x@fastmail.com", wantRoot: "jane@fastmail.com", wantDomain: "fastmail.com"},
		{name: "fastmail fm preserved", input: "jane@fastmail.fm", wantAddr: "jane@fastmail.fm", wantRoot: "jane@fastmail.fm", wantDomain: "fastmail.fm"},

		// ── J. ProtonMail / PM (per-TLD preserved) ───────────────────────
		{name: "proton.me plus stripped", input: "jane+x@proton.me", wantAddr: "jane+x@proton.me", wantRoot: "jane@proton.me", wantDomain: "proton.me"},
		{name: "protonmail.com self root", input: "jane@protonmail.com", wantAddr: "jane@protonmail.com", wantRoot: "jane@protonmail.com", wantDomain: "protonmail.com"},
		{name: "pm.me plus stripped", input: "jane+x@pm.me", wantAddr: "jane+x@pm.me", wantRoot: "jane@pm.me", wantDomain: "pm.me"},

		// ── K. default provider (plus stripped, dots preserved) ──────────
		{name: "default lowercase only", input: "Jane@example.com", wantAddr: "jane@example.com", wantRoot: "jane@example.com", wantDomain: "example.com"},
		{name: "default plus stripped conservatively", input: "jane+work@example.com", wantAddr: "jane+work@example.com", wantRoot: "jane@example.com", wantDomain: "example.com"},
		{name: "default dots preserved", input: "jane.doe@example.com", wantAddr: "jane.doe@example.com", wantRoot: "jane.doe@example.com", wantDomain: "example.com"},
		{name: "default fully uppercase", input: "JANE@EXAMPLE.COM", wantAddr: "jane@example.com", wantRoot: "jane@example.com", wantDomain: "example.com"},

		// ── L. edge / length ─────────────────────────────────────────────
		{name: "trailing dot in domain stripped", input: "jane@example.com.", wantAddr: "jane@example.com", wantRoot: "jane@example.com", wantDomain: "example.com"},
		{name: "exactly 64-char local accepted", input: oneA64 + "@example.com", wantAddr: oneA64 + "@example.com", wantRoot: oneA64 + "@example.com", wantDomain: "example.com"},
		{name: "exactly 254-char total accepted", input: addr254, wantAddr: addr254, wantRoot: addr254, wantDomain: addr254Domain},

		// ── M. branch coverage: lone-dot, IDN failures, alias fallback, escapes ─
		{name: "lone dot is required error", input: ".", wantErr: true},
		{name: "unquoted invalid IDN domain", input: "user@-.com", wantErr: true},
		{name: "plus-only local falls back to full root", input: "+tag@example.com", wantAddr: "+tag@example.com", wantRoot: "+tag@example.com", wantDomain: "example.com"},
		{name: "quoted escaped quote preserved", input: `"a\"b"@example.com`, wantAddr: `"a\"b"@example.com`, wantRoot: `"a\"b"@example.com`, wantDomain: "example.com", wantQuoted: true},
		{name: "quoted local too long", input: `"` + strings.Repeat("a", 65) + `"@example.com`, wantErr: true},
		{name: "quoted invalid domain", input: `"abc"@-.com`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ne, err := models.NormalizeEmail(tc.input)
			if tc.wantErr {
				require.Error(t, err, "input %q must be rejected", tc.input)
				assert.ErrorIs(t, err, models.ErrValidation, "error must wrap ErrValidation")

				return
			}

			require.NoError(t, err, "input %q must be accepted", tc.input)
			assert.Equal(t, tc.wantAddr, ne.Address, "Address mismatch")
			assert.Equal(t, tc.wantRoot, ne.Root, "Root mismatch")
			assert.Equal(t, tc.wantDomain, ne.Domain, "Domain mismatch")
			assert.Equal(t, tc.wantQuoted, ne.IsQuoted, "IsQuoted mismatch")
		})
	}
}

// TestLookupProviderRule sanity-checks the provider table: known providers
// resolve to their canonical domain and an explicit rule; unknown domains
// fall through to the default rule (plus separator, no dot stripping).
func TestLookupProviderRule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input         string
		wantCanonical string
		wantSeparator rune
		wantStripDots bool
	}{
		{input: "gmail.com", wantCanonical: "gmail.com", wantSeparator: '+', wantStripDots: true},
		{input: "googlemail.com", wantCanonical: "gmail.com", wantSeparator: '+', wantStripDots: true},
		{input: "ymail.com", wantCanonical: "yahoo.com", wantSeparator: '+'},
		{input: "me.com", wantCanonical: "icloud.com", wantSeparator: '+'},
		{input: "mac.com", wantCanonical: "icloud.com", wantSeparator: '+'},
		{input: "outlook.com", wantCanonical: "outlook.com", wantSeparator: '+'},
		{input: "fastmail.fm", wantCanonical: "fastmail.fm", wantSeparator: '+'},
		{input: "proton.me", wantCanonical: "proton.me", wantSeparator: '+'},
		{input: "pm.me", wantCanonical: "pm.me", wantSeparator: '+'},
		{input: "example.com", wantCanonical: "example.com", wantSeparator: '+'},
		{input: "unknown.test", wantCanonical: "unknown.test", wantSeparator: '+'},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			gotCanonical, gotRule := models.LookupProviderRule(tc.input)
			assert.Equal(t, tc.wantCanonical, gotCanonical, "canonical domain mismatch")
			assert.Equal(t, tc.wantSeparator, gotRule.Separator, "separator mismatch")
			assert.Equal(t, tc.wantStripDots, gotRule.StripDots, "StripDots mismatch")
		})
	}
}

// BenchmarkNormalizeEmail measures the contact-dedup hot path. Gmail input
// exercises the alias rules (plus-tag stripping + dot removal) so the benchmark
// reflects the most work-heavy provider branch rather than a trivial case.
func BenchmarkNormalizeEmail(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_, _ = models.NormalizeEmail("Jane.Doe+newsletter@googlemail.com")
	}
}

// FuzzNormalizeEmail proves NormalizeEmail never panics on arbitrary input
// and that successful results carry a non-empty Address containing exactly
// one '@' and a non-empty Domain.
func FuzzNormalizeEmail(f *testing.F) {
	for _, seed := range []string{
		"jane@example.com",
		"  Jane@Example.COM  ",
		`"weird@local"@example.com`,
		"user+tag@gmail.com",
		"@nope",
		"nope@",
		"",
		"@",
		"a@b@c",
		"jane@münich.de",
		strings.Repeat("a", 300) + "@example.com",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got, err := models.NormalizeEmail(in)
		if err != nil {
			return
		}

		if got.Address == "" || got.Domain == "" {
			t.Fatalf("address=%q domain=%q must both be non-empty", got.Address, got.Domain)
		}
		// The canonical Address ends with "@" + Domain (quoted local parts may
		// embed additional '@' characters inside their quotes, so a literal
		// Count(@) check is incorrect — but the suffix invariant always holds).
		if !strings.HasSuffix(got.Address, "@"+got.Domain) {
			t.Fatalf("address=%q must end with @%s", got.Address, got.Domain)
		}
	})
}
