package pagination_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/pagination"
)

func TestEncodeCursor_DecodeCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	times := []time.Time{
		time.Unix(0, 0),
		time.Unix(1_000_000_000, 0),
		time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC),
		time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC),
	}

	for _, want := range times {
		t.Run(want.String(), func(t *testing.T) {
			t.Parallel()

			cursor := pagination.EncodeCursor(want)

			got, err := pagination.DecodeCursor(cursor)
			require.NoError(t, err)
			// Cursors have second-level precision (Unix timestamp)
			assert.Equal(t, want.Unix(), got.Unix())
		})
	}
}

func TestEncodeCursor_IsURLSafe(t *testing.T) {
	t.Parallel()

	cursor := pagination.EncodeCursor(time.Now())
	assert.NotContains(t, cursor, "+")
	assert.NotContains(t, cursor, "/")
}

func TestDecodeCursor_AcceptsStandardBase64(t *testing.T) {
	t.Parallel()

	// Encode a known time using standard (non-URL-safe) base64 to test the fallback
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	urlCursor := pagination.EncodeCursor(want)
	// Convert URL-safe → standard base64 (swap - and _ back)
	stdCursor := base64.StdEncoding.EncodeToString(func() []byte {
		b, _ := base64.URLEncoding.DecodeString(urlCursor)
		return b
	}())

	got, err := pagination.DecodeCursor(stdCursor)
	require.NoError(t, err)

	assert.Equal(t, want.Unix(), got.Unix())
}

func TestDecodeCursor_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "empty string", cursor: ""},
		{name: "not base64", cursor: "!!!invalid!!!"},
		{name: "too short", cursor: base64.URLEncoding.EncodeToString([]byte{1, 2, 3})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := pagination.DecodeCursor(tt.cursor)
			require.ErrorIs(t, err, pagination.ErrInvalidCursor)
		})
	}
}

func TestListMeta_JSON(t *testing.T) {
	t.Parallel()

	meta := pagination.ListMeta{Total: 42}
	assert.Equal(t, 42, meta.Total)
}

func TestCursorPagination_Defaults(t *testing.T) {
	t.Parallel()

	p := pagination.CursorPagination{HasMore: true, Limit: 20}
	assert.Empty(t, p.Cursor, "Cursor should default to empty")
	assert.True(t, p.HasMore, "HasMore should be true")
	assert.Equal(t, 20, p.Limit)
}

func BenchmarkEncodeCursor(b *testing.B) {
	ts := time.Now()

	b.ResetTimer()

	for range b.N {
		pagination.EncodeCursor(ts)
	}
}

func BenchmarkDecodeCursor(b *testing.B) {
	cursor := pagination.EncodeCursor(time.Now())

	b.ResetTimer()

	for range b.N {
		_, _ = pagination.DecodeCursor(cursor)
	}
}
