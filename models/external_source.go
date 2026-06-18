package models

import "time"

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

// ValidateExternalSource validates the ExternalSource fields.
func ValidateExternalSource(_ *ExternalSource) error {
	return nil
}
