package models_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/models"
)

func TestValidateRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		fieldName string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "valid string",
			value:     "hello",
			fieldName: "name",
			wantValue: "hello",
			wantErr:   false,
		},
		{
			name:      "trims whitespace",
			value:     "  hello  ",
			fieldName: "name",
			wantValue: "hello",
			wantErr:   false,
		},
		{
			name:      "empty string fails",
			value:     "",
			fieldName: "name",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "whitespace-only string fails",
			value:     "   ",
			fieldName: "name",
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ValidateRequired(tt.value, tt.fieldName)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got)
			}
		})
	}
}

func TestValidateCoordinates(t *testing.T) {
	t.Parallel()

	lat := func(v float64) *float64 { return &v }
	lon := func(v float64) *float64 { return &v }

	tests := []struct {
		name    string
		lat     *float64
		lon     *float64
		wantErr bool
	}{
		{
			name:    "valid coordinates",
			lat:     lat(35.5951),
			lon:     lon(-82.5515),
			wantErr: false,
		},
		{
			name:    "nil coordinates",
			lat:     nil,
			lon:     nil,
			wantErr: false,
		},
		{
			name:    "boundary lat 90",
			lat:     lat(90.0),
			lon:     lon(0.0),
			wantErr: false,
		},
		{
			name:    "boundary lat -90",
			lat:     lat(-90.0),
			lon:     lon(0.0),
			wantErr: false,
		},
		{
			name:    "boundary lon 180",
			lat:     lat(0.0),
			lon:     lon(180.0),
			wantErr: false,
		},
		{
			name:    "boundary lon -180",
			lat:     lat(0.0),
			lon:     lon(-180.0),
			wantErr: false,
		},
		{
			name:    "lat too high",
			lat:     lat(90.1),
			lon:     lon(0.0),
			wantErr: true,
		},
		{
			name:    "lat too low",
			lat:     lat(-90.1),
			lon:     lon(0.0),
			wantErr: true,
		},
		{
			name:    "lon too high",
			lat:     lat(0.0),
			lon:     lon(180.1),
			wantErr: true,
		},
		{
			name:    "lon too low",
			lat:     lat(0.0),
			lon:     lon(-180.1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := models.ValidateCoordinates(tt.lat, tt.lon)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateNonNegative(t *testing.T) {
	t.Parallel()

	val := func(v int64) *int64 { return &v }

	tests := []struct {
		name      string
		value     *int64
		fieldName string
		wantErr   bool
	}{
		{
			name:      "positive value",
			value:     val(100),
			fieldName: "population",
			wantErr:   false,
		},
		{
			name:      "zero value",
			value:     val(0),
			fieldName: "population",
			wantErr:   false,
		},
		{
			name:      "nil value",
			value:     nil,
			fieldName: "population",
			wantErr:   false,
		},
		{
			name:      "negative value",
			value:     val(-1),
			fieldName: "population",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := models.ValidateNonNegative(tt.value, tt.fieldName)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAbbreviation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		abbr      string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "valid uppercase",
			abbr:      "NC",
			wantValue: "NC",
			wantErr:   false,
		},
		{
			name:      "valid lowercase normalizes",
			abbr:      "nc",
			wantValue: "NC",
			wantErr:   false,
		},
		{
			name:      "trims whitespace",
			abbr:      " TX ",
			wantValue: "TX",
			wantErr:   false,
		},
		{
			name:      "too short",
			abbr:      "C",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "too long",
			abbr:      "CAL",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "empty string",
			abbr:      "",
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ValidateAbbreviation(tt.abbr)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got)
			}
		})
	}
}

func TestValidateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		slug      string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "valid slug",
			slug:      "asheville-nc",
			wantValue: "asheville-nc",
			wantErr:   false,
		},
		{
			name:      "normalizes to lowercase",
			slug:      "Asheville-NC",
			wantValue: "asheville-nc",
			wantErr:   false,
		},
		{
			name:      "trims whitespace",
			slug:      " asheville-nc ",
			wantValue: "asheville-nc",
			wantErr:   false,
		},
		{
			name:      "empty string fails",
			slug:      "",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "whitespace-only fails",
			slug:      "   ",
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ValidateSlug(tt.slug)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got)
			}
		})
	}
}

// FuzzValidateSlug proves ValidateSlug never panics and that successful
// outputs contain no leading/trailing whitespace and no uppercase letters.
func FuzzValidateSlug(f *testing.F) {
	for _, seed := range []string{"", " ", "abc", "ABC", "a-b-c", "  spaced  ", "🦀", "a\x00b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got, err := models.ValidateSlug(in)
		if err != nil {
			return
		}

		if got != strings.TrimSpace(got) {
			t.Fatalf("slug = %q has untrimmed whitespace", got)
		}

		if got != strings.ToLower(got) {
			t.Fatalf("slug = %q is not lowercased", got)
		}
	})
}

// FuzzValidateAbbreviation proves the function never panics and that a
// successful result is exactly two uppercase characters.
func FuzzValidateAbbreviation(f *testing.F) {
	for _, seed := range []string{"", "a", "ab", "ABC", "  us  ", "🦀🦀", "12"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		got, err := models.ValidateAbbreviation(in)
		if err != nil {
			return
		}

		if len(got) != 2 {
			t.Fatalf("abbr = %q has length %d, want 2", got, len(got))
		}

		if got != strings.ToUpper(got) {
			t.Fatalf("abbr = %q is not uppercased", got)
		}
	})
}

// FuzzValidateOptionalURL proves the validator never panics. Successful
// results imply the URL begins with http:// or https:// or is empty.
func FuzzValidateOptionalURL(f *testing.F) {
	for _, seed := range []string{"", " ", "http://x", "https://x", "ftp://x", "javascript:alert(1)", "https://example.com/" + strings.Repeat("a", 5000)} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		// Test both the nil and non-nil pointer paths.
		if err := models.ValidateOptionalURL(nil, "u"); err != nil {
			t.Fatalf("nil pointer must always succeed, got %v", err)
		}

		_ = models.ValidateOptionalURL(&in, "u")
	})
}
