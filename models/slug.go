package models

import (
	"regexp"
	"strings"
)

// slugNonSlugChars matches runs of characters that are not lowercase
// alphanumerics or hyphens; they are stripped from a slug.
var slugNonSlugChars = regexp.MustCompile(`[^a-z0-9-]+`)

// slugConsecutiveDashes matches runs of hyphens, collapsed to a single hyphen.
var slugConsecutiveDashes = regexp.MustCompile(`-+`)

// GenerateSlug creates a URL-friendly slug from the given string.
// It converts to lowercase, replaces spaces/underscores with hyphens,
// removes non-alphanumeric characters, and cleans up consecutive hyphens.
func GenerateSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and underscores with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove non-alphanumeric characters except hyphens
	s = slugNonSlugChars.ReplaceAllString(s, "")

	// Remove consecutive hyphens
	s = slugConsecutiveDashes.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	return s
}
