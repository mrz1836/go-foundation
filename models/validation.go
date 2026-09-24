package models

import "strings"

// msgRequired is the shared validation message for an empty required field.
const msgRequired = "is required"

// Latitude/longitude bounds (WGS84 degrees) shared by the coordinate validators.
const (
	minLatitude  = -90.0
	maxLatitude  = 90.0
	minLongitude = -180.0
	maxLongitude = 180.0
)

// validateLatLon validates a latitude/longitude pair against the WGS84 bounds,
// returning a ValidationError naming the first out-of-range field.
func validateLatLon(lat, lon float64) error {
	if lat < minLatitude || lat > maxLatitude {
		return NewValidationError("latitude", "must be between -90 and 90")
	}

	if lon < minLongitude || lon > maxLongitude {
		return NewValidationError("longitude", "must be between -180 and 180")
	}

	return nil
}

// ValidateRequired trims and validates a required string field.
// Returns the trimmed value and a validation error if empty.
func ValidateRequired(value, fieldName string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", NewValidationError(fieldName, msgRequired)
	}

	return trimmed, nil
}

// ValidateCoordinates validates latitude and longitude if provided.
// Latitude must be between -90 and 90, longitude between -180 and 180.
// A nil pointer is treated as unset (and therefore valid).
func ValidateCoordinates(lat, lon *float64) error {
	la, lo := 0.0, 0.0 // 0,0 is in range, so an unset field never trips the check
	if lat != nil {
		la = *lat
	}
	if lon != nil {
		lo = *lon
	}

	return validateLatLon(la, lo)
}

// ValidateNonNegative validates an int64 pointer is non-negative.
func ValidateNonNegative(value *int64, fieldName string) error {
	if value != nil && *value < 0 {
		return NewValidationError(fieldName, "must be non-negative")
	}

	return nil
}

// ValidateAbbreviation validates and normalizes a 2-character abbreviation.
// Returns the normalized uppercase abbreviation.
func ValidateAbbreviation(abbr string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(abbr))
	if len(normalized) != 2 {
		return "", NewValidationError("abbreviation", "must be exactly 2 characters")
	}

	return normalized, nil
}

// ValidateSlug normalizes and validates a slug.
// Returns the normalized lowercase slug.
func ValidateSlug(slug string) (string, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return "", NewValidationError("slug", msgRequired)
	}

	return slug, nil
}

// ValidateOptionalUUID validates an optional UUID pointer field.
// Returns nil if the UUID is nil/empty or valid, validation error if invalid.
func ValidateOptionalUUID(uuidPtr *string, fieldName string) error {
	if uuidPtr == nil || *uuidPtr == "" {
		return nil
	}

	if err := ValidateUUID(*uuidPtr); err != nil {
		return NewValidationError(fieldName, "invalid UUID format")
	}

	return nil
}

// ValidateOptionalURL validates an optional URL field.
// Returns nil if the URL is nil/empty or valid, error if invalid.
func ValidateOptionalURL(urlStr *string, fieldName string) error {
	if urlStr == nil || *urlStr == "" {
		return nil
	}

	trimmed := strings.TrimSpace(*urlStr)
	if trimmed == "" {
		return nil
	}
	// Basic URL validation: must start with http:// or https://
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return NewValidationError(fieldName, "must be a valid URL starting with http:// or https://")
	}

	return nil
}
