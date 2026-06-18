package models

import (
	"strings"
	"time"
)

// External source column limits, mirroring the `gorm:"size:..."` tags so
// validation rejects values the database would truncate.
const (
	maxProviderLength   = 50
	maxExternalIDLength = 500
)

// ExternalSource is an embeddable struct that provides provenance tracking
// for entities created by external data providers. All fields are optional
// (pointers) so models can be either user-created (nil provider) or
// externally-sourced.
//
// Embed anonymously in models so fields are promoted (e.g., game.Provider):
//
//	type League struct {
//	    models.BaseModel
//	    models.ExternalSource
//	    Name string `gorm:"size:255;not null" json:"name"`
//	}
type ExternalSource struct {
	// Provider is the external provider name (e.g., "leaguelinq").
	Provider *string `gorm:"size:50;index" json:"provider,omitempty"`

	// ExternalID is the provider-assigned unique identifier used as a dedup key.
	ExternalID *string `gorm:"size:500" json:"external_id,omitempty"`

	// LastSeenAt tracks the last time this entity was observed from the provider.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// IsExternal returns true if this entity was created by an external provider.
func (e *ExternalSource) IsExternal() bool {
	return e.Provider != nil && *e.Provider != ""
}

// ValidateExternalSource validates the ExternalSource fields against the
// storage constraints and the dedup-key contract:
//
//   - Provider (when set) must be at most 50 characters.
//   - ExternalID (when set) must be at most 500 characters.
//   - When Provider is set, ExternalID is required: the two together form the
//     provider dedup key, so a provider without an external identifier cannot be
//     deduplicated.
//
// A nil receiver and a fully-empty source are both valid (user-created entities
// carry no provenance). Returns a ValidationError wrapping ErrValidation.
func ValidateExternalSource(e *ExternalSource) error {
	if e == nil {
		return nil
	}

	provider := trimPtr(e.Provider)
	externalID := trimPtr(e.ExternalID)

	if len(provider) > maxProviderLength {
		return NewValidationError("provider", "must be at most 50 characters")
	}

	if len(externalID) > maxExternalIDLength {
		return NewValidationError("external_id", "must be at most 500 characters")
	}

	if provider != "" && externalID == "" {
		return NewValidationError("external_id", "is required when provider is set")
	}

	return nil
}

// trimPtr returns the whitespace-trimmed value of a string pointer, or "" when
// the pointer is nil.
func trimPtr(s *string) string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(*s)
}
