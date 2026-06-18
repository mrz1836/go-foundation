package models

// CosApprox provides a simple cosine approximation for geo calculations.
// Uses Taylor series approximation for small angles.
func CosApprox(degrees float64) float64 {
	// Convert to radians
	rad := degrees * 0.017453292519943295 // pi/180

	// Taylor series approximation: cos(x) ≈ 1 - x²/2 + x⁴/24
	x2 := rad * rad

	return 1 - x2/2 + x2*x2/24
}

// BoundingBox calculates lat/lng bounds for a center point and radius.
// Returns (minLat, maxLat, minLon, maxLon).
// Uses the approximation that 1 degree latitude ≈ 111km.
func BoundingBox(lat, lon, radiusKm float64) (minLat, maxLat, minLon, maxLon float64) {
	// 1 degree latitude ≈ 111km
	latDelta := radiusKm / 111.0
	// 1 degree longitude varies by latitude
	lonDelta := radiusKm / (111.0 * CosApprox(lat))

	return lat - latDelta, lat + latDelta, lon - lonDelta, lon + lonDelta
}

// ValidateGeoParams validates geo query parameters.
func ValidateGeoParams(lat, lon, radiusKm float64) error {
	if lat < -90 || lat > 90 {
		return NewValidationError("latitude", "must be between -90 and 90")
	}

	if lon < -180 || lon > 180 {
		return NewValidationError("longitude", "must be between -180 and 180")
	}

	if radiusKm <= 0 {
		return NewValidationError("radius", "must be positive")
	}

	return nil
}
