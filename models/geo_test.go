package models_test

import (
	"math"
	"testing"

	"github.com/mrz1836/go-foundation/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCosApprox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		degrees float64
		want    float64
		delta   float64 // allowed error
	}{
		{
			name:    "zero degrees",
			degrees: 0.0,
			want:    1.0,
			delta:   0.0001,
		},
		{
			name:    "45 degrees",
			degrees: 45.0,
			want:    math.Cos(45.0 * math.Pi / 180.0),
			delta:   0.01,
		},
		{
			name:    "90 degrees",
			degrees: 90.0,
			want:    0.0,
			delta:   0.1, // Taylor approx less accurate at 90
		},
		{
			name:    "negative degrees",
			degrees: -30.0,
			want:    math.Cos(-30.0 * math.Pi / 180.0),
			delta:   0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := models.CosApprox(tt.degrees)
			assert.InDelta(t, tt.want, got, tt.delta)
		})
	}
}

func TestBoundingBox(t *testing.T) {
	t.Parallel()

	// Test with known values
	// Asheville: 35.5951, -82.5515
	// 10km radius

	lat, lon, radius := 35.5951, -82.5515, 10.0

	minLat, maxLat, minLon, maxLon := models.BoundingBox(lat, lon, radius)

	// 1 degree lat ≈ 111km, so 10km ≈ 0.09 degrees
	expectedLatDelta := radius / 111.0

	assert.InDelta(t, lat-expectedLatDelta, minLat, 0.001)
	assert.InDelta(t, lat+expectedLatDelta, maxLat, 0.001)

	// Longitude delta should be adjusted for latitude
	// At 34 degrees, cos(34) ≈ 0.829
	expectedLonDelta := radius / (111.0 * models.CosApprox(lat))

	assert.InDelta(t, lon-expectedLonDelta, minLon, 0.001)
	assert.InDelta(t, lon+expectedLonDelta, maxLon, 0.001)

	// Sanity checks
	assert.Less(t, minLat, maxLat)
	assert.Less(t, minLon, maxLon)
}

func TestValidateGeoParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lat      float64
		lon      float64
		radiusKm float64
		wantErr  bool
	}{
		{
			name:     "valid params",
			lat:      35.5951,
			lon:      -82.5515,
			radiusKm: 10.0,
			wantErr:  false,
		},
		{
			name:     "lat too high",
			lat:      91.0,
			lon:      0.0,
			radiusKm: 10.0,
			wantErr:  true,
		},
		{
			name:     "lat too low",
			lat:      -91.0,
			lon:      0.0,
			radiusKm: 10.0,
			wantErr:  true,
		},
		{
			name:     "lon too high",
			lat:      0.0,
			lon:      181.0,
			radiusKm: 10.0,
			wantErr:  true,
		},
		{
			name:     "lon too low",
			lat:      0.0,
			lon:      -181.0,
			radiusKm: 10.0,
			wantErr:  true,
		},
		{
			name:     "zero radius",
			lat:      0.0,
			lon:      0.0,
			radiusKm: 0.0,
			wantErr:  true,
		},
		{
			name:     "negative radius",
			lat:      0.0,
			lon:      0.0,
			radiusKm: -10.0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := models.ValidateGeoParams(tt.lat, tt.lon, tt.radiusKm)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, models.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
