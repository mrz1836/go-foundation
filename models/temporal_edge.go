package models

import (
	"time"

	"gorm.io/gorm"
)

// The conventional TemporalEdge.VerificationStatus values. The storage layer
// does not enforce the set; writers use these constants.
const (
	VerificationStatusUnverified = "unverified" // nothing has verified the value; the default
	VerificationStatusVerified   = "verified"   // a verification step confirmed the value
	VerificationStatusDisputed   = "disputed"   // a source or reviewer contests the value
	VerificationStatusSuppressed = "suppressed" // a verification outcome that withholds the value; not SuppressedAt
)

// ActiveEdgePredicate is the SQL condition that selects active edges: not
// ended, not superseded, and not suppressed. Its column names are
// unqualified; qualify them when joining. A partial "active" index on an edge
// table should use the same condition, so queries that filter on it can use
// the index.
const ActiveEdgePredicate = "valid_to IS NULL AND superseded_by_id IS NULL AND suppressed_at IS NULL"

// TemporalEdge is the substrate for time-bounded relationships. Embed the
// typed form in any edge model (ownership, lien, mailing intent, communication,
// agent decision). It carries real-world validity, system time, source event
// time, append-only correction fields, and inline provenance.
//
// TemporalEdge is generic over its own ID type so each edge model gets a
// distinct typed ID and a typed SupersededByID self-reference:
//
//	type PartyPhone struct {
//	    models.TemporalEdge[PartyPhoneID]
//	    PersonID people.PersonID
//	    PhoneID  PhoneID
//	}
//
// TemporalEdge does NOT embed BaseModel — entities and relationships are
// independent substrates. Corrections are append-only (SupersededByID,
// SuppressedAt), so it has no soft-delete and no metadata blob.
//
// Change an edge's lifecycle only with SupersedeEdge, EndEdge, or
// SuppressEdge, never by updating its columns.
type TemporalEdge[ID ~string] struct {
	// ID is the UUID v7 primary key, minted on create when empty.
	ID ID `gorm:"type:uuid;primaryKey" json:"id"`

	// ValidFrom is the real-world start of the relationship. It is never
	// auto-defaulted — a zero value signals a writer defect.
	ValidFrom time.Time `json:"valid_from"`

	// ValidTo is the real-world end; nil means the edge is still active.
	ValidTo *time.Time `json:"valid_to,omitempty"`

	// RecordedAt is the system time the edge was written; auto-defaulted to
	// the context clock's now when zero.
	RecordedAt time.Time `json:"recorded_at"`

	// EventTime is the source-asserted real-world event time; nil when the
	// source publishes none.
	EventTime *time.Time `json:"event_time,omitempty"`

	// SupersededByID points to the row that replaces this one: a correction
	// (SupersedeEdge) or a closed copy that records the edge's end (EndEdge).
	SupersededByID *ID `gorm:"type:uuid" json:"superseded_by_id,omitempty"`

	// SuppressedAt is set by SuppressEdge when the edge is taken down.
	SuppressedAt *time.Time `json:"suppressed_at,omitempty"`

	// Source is the human-readable adapter/source name.
	Source string `json:"source"`

	// ByteHash is the SHA-256 of what the edge derives from: the source bytes
	// (or byte range) for a fetched fact, or the normalized value for one
	// entered or proven first-hand. It may be empty when a hash would reveal
	// the value.
	ByteHash string `json:"byte_hash"`

	// RuleVersion is the version of the derivation rule that produced the edge.
	RuleVersion string `json:"rule_version"`

	// PipelineVersionHash is sha256(adapter_version‖rule_version‖fixture_version).
	PipelineVersionHash string `json:"pipeline_version_hash"`

	// Confidence is the source-published confidence; nil means not set, which
	// is distinct from a real 0.0 confidence.
	Confidence *float64 `json:"confidence,omitempty"`

	// VerificationStatus defaults to VerificationStatusUnverified when written
	// empty.
	VerificationStatus string `json:"verification_status"`

	// CreatedAt is the row insert time; equals RecordedAt unless backdated by
	// replay tooling. Auto-defaulted to the context clock's now when zero.
	CreatedAt time.Time `json:"created_at"`
}

// Edge returns e. A model that embeds TemporalEdge inherits it, which lets the
// generic lifecycle helpers reach the edge fields of any such model.
func (e *TemporalEdge[ID]) Edge() *TemporalEdge[ID] { return e }

// BeforeCreate is the GORM hook that mints the ID and applies defaults. An
// explicitly-set ID, RecordedAt, or CreatedAt is preserved so replay tooling
// can supply its own. ValidFrom is never defaulted.
func (e *TemporalEdge[ID]) BeforeCreate(tx *gorm.DB) error {
	e.ID = mintID(e.ID)

	if e.RecordedAt.IsZero() || e.CreatedAt.IsZero() {
		ctx := tx.Statement.Context

		now := ClockFrom(ctx).Now(ctx)
		if e.RecordedAt.IsZero() {
			e.RecordedAt = now
		}

		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
	}

	if e.VerificationStatus == "" {
		e.VerificationStatus = VerificationStatusUnverified
	}

	return nil
}
