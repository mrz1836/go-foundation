package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// BaseModel provides common fields for all models in the system. It is
// generic over a typed ID (any `~string` alias) so that each bounded context
// can declare its own ID type (e.g. people.PersonID, channels.PhoneID) without
// losing compile-time safety.
//
// Embed the typed form:
//
//	type Person struct {
//	    models.BaseModel[PersonID]
//	    GivenName  string
//	}
//
// # PostgreSQL vs SQLite Considerations
//
// This package is designed to work with both PostgreSQL (production) and SQLite
// (testing). Key compatibility considerations:
//
// ## UUID Type
//
// PostgreSQL has native UUID type support, while SQLite stores UUIDs as TEXT.
// Typed IDs (e.g. PersonID) are string aliases — GORM's reflection sees the
// underlying string and serializes correctly on both drivers.
//
// ## JSONB Type
//
// The datatypes.JSON type handles cross-database JSON storage:
//   - PostgreSQL: Uses JSONB column type with indexing support
//   - SQLite: Uses TEXT column with JSON stored as string
//
// ## Timestamps
//
// GORM's automatic timestamp handling works on both databases.
//
// ## Soft Delete Filtering
//
// The gorm.DeletedAt type provides automatic soft-delete filtering on both.
type BaseModel[ID ~string] struct {
	// ID is the UUID primary key, auto-generated on create if empty.
	ID ID `gorm:"type:uuid;primaryKey" json:"id"`

	// CreatedAt is automatically set when the record is created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is automatically updated when the record is modified.
	UpdatedAt time.Time `json:"updated_at"`

	// DeletedAt enables soft-delete functionality - records are filtered by default.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Metadata allows storing arbitrary JSON data for extensibility.
	Metadata datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
}

// BeforeCreate is a GORM hook that generates a UUID v7 for the ID field if not
// already set. v7 IDs are time-ordered; an explicitly-set ID is preserved
// unchanged so replay tooling can supply its own identifiers.
func (b *BaseModel[ID]) BeforeCreate(_ *gorm.DB) error {
	if b.ID == "" {
		b.ID = ID(NewID())
	}

	return nil
}

// IsDeleted returns true if the record has been soft-deleted.
func (b *BaseModel[ID]) IsDeleted() bool {
	return b.DeletedAt.Valid
}

// GetID returns the model's ID as a string. Satisfies the [Auditable]
// interface so audit hooks can log a uniform string identifier across all
// typed-ID models.
func (b *BaseModel[ID]) GetID() string {
	return string(b.ID)
}
