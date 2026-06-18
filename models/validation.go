package models

import "strings"

// ValidateRequired trims and validates a required string field.
// Returns the trimmed value and a validation error if empty.
func ValidateRequired(value, fieldName string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", NewValidationError(fieldName, "is required")
	}

	return trimmed, nil
}

// ValidateCoordinates validates latitude and longitude if provided.
// Latitude must be between -90 and 90, longitude between -180 and 180.
func ValidateCoordinates(lat, lon *float64) error {
	if lat != nil && (*lat < -90 || *lat > 90) {
		return NewValidationError("latitude", "must be between -90 and 90")
	}

	if lon != nil && (*lon < -180 || *lon > 180) {
		return NewValidationError("longitude", "must be between -180 and 180")
	}

	return nil
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
		return "", NewValidationError("slug", "is required")
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
