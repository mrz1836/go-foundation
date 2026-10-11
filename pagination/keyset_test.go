package pagination_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/pagination"
)

// keysetTestTime is a fixed instant with nanoseconds, in UTC.
func keysetTestTime() time.Time {
	return time.Date(2026, 3, 1, 12, 0, 0, 123456789, time.UTC)
}

func TestEncodeKeyset_DecodeKeyset_RoundTrip(t *testing.T) {
	t.Parallel()

	minusFive := time.FixedZone("UTC-5", -5*60*60)

	tests := []struct {
		name string
		at   time.Time
		id   string
	}{
		{name: "nanoseconds in UTC", at: keysetTestTime(), id: "row-1"},
		{name: "another zone", at: time.Date(2026, 3, 1, 12, 0, 0, 500_000_000, minusFive), id: "row-2"},
		{name: "the Unix epoch", at: time.Unix(0, 0), id: "row-3"},
		{name: "before 1970", at: time.Date(1969, 12, 31, 23, 59, 59, 500_000_000, time.UTC), id: "row-4"},
		{name: "the zero time", at: time.Time{}, id: "row-5"},
		{name: "the year 9999", at: time.Date(9999, 12, 31, 23, 59, 59, 999_999_999, time.UTC), id: "row-6"},
		{name: "an empty id", at: keysetTestTime(), id: ""},
		{name: "a UUID", at: keysetTestTime(), id: "0192f3c4-7d1e-7a2b-8c3d-4e5f60718293"},
		{name: "the longest id", at: keysetTestTime(), id: strings.Repeat("x", 753)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k, err := pagination.DecodeKeyset(pagination.EncodeKeyset(tt.at, tt.id))
			require.NoError(t, err)
			assert.True(t, tt.at.Equal(k.At), "got %v, want %v", k.At, tt.at)
			assert.Equal(t, time.UTC, k.At.Location())
			assert.Equal(t, tt.id, k.ID)
			assert.False(t, k.Legacy)
		})
	}
}

func TestEncodeKeyset_WritesVersionOne(t *testing.T) {
	t.Parallel()

	at := keysetTestTime()
	cursor := pagination.EncodeKeyset(at, "row-1")

	assert.Equal(t, base64.RawURLEncoding.EncodeToString(versionOnePayload(at, "row-1")), cursor)
	assert.NotContains(t, cursor, "=")
	assert.NotContains(t, cursor, "+")
	assert.NotContains(t, cursor, "/")
}

func TestDecodeKeyset_AcceptsEveryBase64Form(t *testing.T) {
	t.Parallel()

	at := keysetTestTime()
	payload := versionOnePayload(at, "~~~~~")

	urlPadded := base64.URLEncoding.EncodeToString(payload)
	require.True(t, strings.ContainsAny(urlPadded, "-_"), "the payload must exercise the URL-safe alphabet: %s", urlPadded)
	require.True(t, strings.HasSuffix(urlPadded, "="), "the payload must need padding: %s", urlPadded)

	forms := map[string]string{
		"URL-safe, unpadded": base64.RawURLEncoding.EncodeToString(payload),
		"URL-safe, padded":   urlPadded,
		"standard, unpadded": base64.RawStdEncoding.EncodeToString(payload),
		"standard, padded":   base64.StdEncoding.EncodeToString(payload),
	}

	for name, cursor := range forms {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			k, err := pagination.DecodeKeyset(cursor)
			require.NoError(t, err)
			assert.True(t, at.Equal(k.At), "got %v, want %v", k.At, at)
			assert.Equal(t, "~~~~~", k.ID)
			assert.False(t, k.Legacy)
		})
	}
}

func TestDecodeKeyset_ReadsALegacyCursorAsALowerBound(t *testing.T) {
	t.Parallel()

	legacy := pagination.EncodeCursor(time.Date(2026, 3, 1, 12, 0, 0, 900_000_000, time.UTC))
	raw, err := base64.URLEncoding.DecodeString(legacy)
	require.NoError(t, err)

	forms := map[string]string{
		"as written":          legacy,
		"in the std alphabet": base64.StdEncoding.EncodeToString(raw),
	}
	want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	for name, cursor := range forms {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			k, decodeErr := pagination.DecodeKeyset(cursor)
			require.NoError(t, decodeErr)
			assert.True(t, want.Equal(k.At), "got %v, want %v", k.At, want)
			assert.Equal(t, time.UTC, k.At.Location())
			assert.Empty(t, k.ID)
			assert.True(t, k.Legacy)
		})
	}
}

func TestDecodeKeyset_RefusesInvalidCursors(t *testing.T) {
	t.Parallel()

	valid := versionOnePayload(keysetTestTime(), "row-1")
	header := valid[:13]

	withVersion := append([]byte{0x02}, valid[1:]...)
	nanosOutOfRange := append([]byte{}, valid...)
	copy(nanosOutOfRange[9:13], []byte{0x3B, 0x9A, 0xCA, 0x00}) // 1,000,000,000

	raw := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	join := func(parts ...[]byte) []byte {
		var out []byte
		for _, p := range parts {
			out = append(out, p...)
		}

		return out
	}

	tests := []struct {
		name   string
		cursor string
		reason string
	}{
		{name: "empty", cursor: "", reason: "empty"},
		{name: "not base64", cursor: "!!!", reason: "bad encoding"},
		{name: "unknown version", cursor: raw(withVersion), reason: "unknown version"},
		{name: "truncated time", cursor: raw([]byte{0x01, 0, 0, 0, 0, 0}), reason: "truncated time"},
		{name: "truncated id length", cursor: raw(header), reason: "truncated id length"},
		{name: "id length overflows", cursor: raw(join(header, []byte(strings.Repeat("\xff", 11)))), reason: "id length overflows"},
		{name: "id length not minimal", cursor: raw(join(header, []byte{0x85, 0x00}, []byte("row-1"))), reason: "id length not minimal"},
		{name: "truncated id", cursor: raw(join(header, []byte{5}, []byte("ro"))), reason: "truncated id"},
		{name: "trailing bytes", cursor: raw(join(valid, []byte{0})), reason: "trailing bytes"},
		{name: "a 9-byte payload", cursor: base64.URLEncoding.EncodeToString(make([]byte, 9)), reason: "unknown version"},
		{name: "nanoseconds out of range", cursor: raw(nanosOutOfRange), reason: "nanoseconds out of range"},
		{name: "an id too long to encode", cursor: pagination.EncodeKeyset(keysetTestTime(), strings.Repeat("x", 754)), reason: "longer than 1024 characters"},
		{name: "longer than 1024 characters", cursor: strings.Repeat("A", 1025), reason: "longer than 1024 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := pagination.DecodeKeyset(tt.cursor)
			require.ErrorIs(t, err, pagination.ErrInvalidCursor)
			assert.Equal(t, "pagination: invalid cursor: "+tt.reason, err.Error())

			if tt.cursor != "" {
				assert.NotContains(t, err.Error(), tt.cursor, "the error must never echo the cursor")
			}
		})
	}
}

func TestKeyset_IsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, pagination.Keyset{}.IsZero())
	assert.False(t, pagination.Keyset{At: time.Unix(0, 0).UTC(), Legacy: true}.IsZero(), "a legacy position at the epoch")
	assert.False(t, pagination.Keyset{ID: "row-1"}.IsZero(), "only an id")
	assert.False(t, pagination.Keyset{At: keysetTestTime()}.IsZero(), "only a time")
}

func BenchmarkEncodeKeyset(b *testing.B) {
	at := keysetTestTime()
	id := "0192f3c4-7d1e-7a2b-8c3d-4e5f60718293"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		pagination.EncodeKeyset(at, id)
	}
}

func BenchmarkDecodeKeyset(b *testing.B) {
	cursor := pagination.EncodeKeyset(keysetTestTime(), "0192f3c4-7d1e-7a2b-8c3d-4e5f60718293")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, _ = pagination.DecodeKeyset(cursor)
	}
}
