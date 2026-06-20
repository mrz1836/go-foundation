package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSplitLocalDomain exercises the unexported splitLocalDomain helper,
// including the ok=false guards (missing, leading, or trailing "@") that are
// unreachable from NormalizeEmail because net/mail.ParseAddress rejects those
// shapes before splitLocalDomain is ever reached.
func TestSplitLocalDomain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		addr       string
		wantLocal  string
		wantDomain string
		wantOK     bool
	}{
		{name: "valid split", addr: "a@b", wantLocal: "a", wantDomain: "b", wantOK: true},
		{name: "no at sign", addr: "noatsign", wantOK: false},
		{name: "leading at sign", addr: "@domain", wantOK: false},
		{name: "trailing at sign", addr: "local@", wantOK: false},
		{name: "only at sign", addr: "@", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			local, domain, ok := splitLocalDomain(tc.addr)
			assert.Equal(t, tc.wantOK, ok, "ok mismatch")
			assert.Equal(t, tc.wantLocal, local, "local mismatch")
			assert.Equal(t, tc.wantDomain, domain, "domain mismatch")
		})
	}
}

// TestClosingQuoteIndex exercises closingQuoteIndex, including the unterminated
// (-1) path that NormalizeEmail cannot reach because net/mail rejects an
// unterminated quoted string before normalizeQuoted inspects it.
func TestClosingQuoteIndex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{name: "simple terminated", in: `"abc"`, want: 4},
		{name: "escaped quote then close", in: `"a\""`, want: 4},
		{name: "unterminated returns -1", in: `"abc`, want: -1},
		{name: "trailing escape unterminated", in: `"a\"`, want: -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, closingQuoteIndex(tc.in))
		})
	}
}

// TestCanonicalDomain exercises canonicalDomain's empty-domain guard, which
// NormalizeEmail cannot reach because the parse and split stages reject empty
// domains earlier.
func TestCanonicalDomain(t *testing.T) {
	t.Parallel()

	t.Run("empty after trim and trailing dot", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{"", ".", "  .  "} {
			got, err := canonicalDomain(in)
			require.ErrorIs(t, err, ErrValidation, "input %q must error", in)
			assert.Empty(t, got)
		}
	})

	t.Run("valid domain canonicalized", func(t *testing.T) {
		t.Parallel()

		got, err := canonicalDomain("  Example.COM.  ")
		require.NoError(t, err)
		assert.Equal(t, "example.com", got)
	})
}

// TestBuildUnquoted exercises buildUnquoted's defensive guards directly. These
// guards sit behind net/mail.ParseAddress in normalizeUnquoted, so crafted
// inputs (which the parser would reject) are required to reach them.
func TestBuildUnquoted(t *testing.T) {
	t.Parallel()

	t.Run("split failure rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildUnquoted("noatsign")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("invalid domain rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildUnquoted("user@-.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("assembled address over 254 rejected", func(t *testing.T) {
		t.Parallel()

		// 64-char local + "@" + a domain long enough to exceed 254 total. Each
		// label stays within the 63-char DNS limit so canonicalDomain accepts it.
		local := strings.Repeat("a", 64)
		domain := strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + ".co"
		_, err := buildUnquoted(local + "@" + domain)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("valid address assembled", func(t *testing.T) {
		t.Parallel()

		ne, err := buildUnquoted("Jane@Example.com")
		require.NoError(t, err)
		assert.Equal(t, "jane@example.com", ne.Address)
		assert.False(t, ne.IsQuoted)
	})
}

// TestBuildQuoted exercises buildQuoted's defensive guards directly. These
// guards sit behind net/mail.ParseAddress in normalizeQuoted, so crafted
// inputs (which the parser would reject) are required to reach them.
func TestBuildQuoted(t *testing.T) {
	t.Parallel()

	t.Run("closing quote not followed by at sign", func(t *testing.T) {
		t.Parallel()

		_, err := buildQuoted(`"a b"x@example.com`)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("unterminated quote", func(t *testing.T) {
		t.Parallel()

		_, err := buildQuoted(`"abc`)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("empty quoted local rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildQuoted(`""@example.com`)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("quoted local over 64 rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildQuoted(`"` + strings.Repeat("a", 65) + `"@example.com`)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("invalid domain rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildQuoted(`"abc"@-.com`)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("assembled address over 254 rejected", func(t *testing.T) {
		t.Parallel()

		local := `"` + strings.Repeat("a", 62) + `"` // 64 chars including quotes
		domain := strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + "." + strings.Repeat("a", 63) + ".co"
		_, err := buildQuoted(local + "@" + domain)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("valid quoted address assembled", func(t *testing.T) {
		t.Parallel()

		ne, err := buildQuoted(`"john doe"@Example.com`)
		require.NoError(t, err)
		assert.Equal(t, `"john doe"@example.com`, ne.Address)
		assert.True(t, ne.IsQuoted)
	})
}
