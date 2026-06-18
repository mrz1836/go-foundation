package models

import (
	"strings"

	"gorm.io/gorm"
)

// SearchByNameOpt returns a query option for case-insensitive name search.
// If the query is empty, returns a no-op option.
func SearchByNameOpt(query string) QueryOption {
	query = strings.TrimSpace(query)
	if query == "" {
		return func(db *gorm.DB) *gorm.DB { return db }
	}

	return WithCondition("LOWER(name) LIKE LOWER(?)", "%"+query+"%")
}

// FindBySlugOpt returns a query option for slug lookup.
// Returns an error if the slug is empty after normalization.
func FindBySlugOpt(slug string) (QueryOption, error) {
	slug, err := ValidateSlug(slug)
	if err != nil {
		return nil, err
	}

	return WithCondition("slug = ?", slug), nil
}

// FindNearbyOpt returns a query option for bounding box geo queries.
// Validates coordinates and calculates the bounding box.
func FindNearbyOpt(lat, lon, radiusKm float64) (QueryOption, error) {
	if err := ValidateGeoParams(lat, lon, radiusKm); err != nil {
		return nil, err
	}

	minLat, maxLat, minLon, maxLon := BoundingBox(lat, lon, radiusKm)

	return WithCondition(
		"latitude IS NOT NULL AND longitude IS NOT NULL AND latitude BETWEEN ? AND ? AND longitude BETWEEN ? AND ?",
		minLat, maxLat, minLon, maxLon,
	), nil
}
