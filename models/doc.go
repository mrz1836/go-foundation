// Package models provides core infrastructure for GORM-based domain models.
//
// This package contains the building blocks that all domain models depend on:
//   - BaseModel: Embedded struct providing ID, timestamps, soft-delete, metadata
//   - Repository[T]: Generic CRUD repository with query options
//   - Trait interfaces: Sluggable, GeoLocatable, Nameable, Auditable
//   - Errors: Sentinel errors and ValidationError for consistent error handling
//   - Hooks: Lifecycle hook system for audit logging and extensibility
//
// # Usage
//
// Domain models embed BaseModel and implement desired traits:
//
//	type State struct {
//	    models.BaseModel
//	    Name         string `gorm:"size:100;not null"`
//	    Abbreviation string `gorm:"size:2;uniqueIndex"`
//	}
//
//	func (s *State) GetName() string { return s.Name }  // Implements Nameable
//
// Repositories embed the generic Repository:
//
//	type stateRepository struct {
//	    *models.Repository[State]
//	}
//
// # Query Options
//
// Use QueryOption functions to build flexible queries:
//
//	states, err := repo.FindAll(ctx,
//	    models.WithLimit(10),
//	    models.WithOrderBy("name", false),
//	    models.WithPreload("Cities"),
//	)
//
// # Temporal edges
//
// TemporalEdge is append-only. Change an edge only with SupersedeEdge (correct
// it), EndEdge (record its end with a closed copy), or SuppressEdge (take it
// down), inside a transaction (see Transactor). Each acts only on an active
// edge (ActiveEdgePredicate) that matches the id and the caller's EdgeWhere
// conditions, and returns ErrNotFound otherwise. On any error, roll the
// transaction back; WithinTx does when its callback returns the error:
//
//	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
//	    _, err := models.EndEdge[PartyPhone](ctx, edgeID, models.EdgeWhere("person_id = ?", personID))
//	    return err
//	})
//
// The superseded_by_id column must carry no immediate foreign key to its own
// table (none, or one declared DEFERRABLE INITIALLY DEFERRED), because a
// supersede points it at the row it inserts next. The VerificationStatus
// constants (VerificationStatusUnverified and the rest) are the conventional
// statuses.
//
// # Normalizers
//
// NormalizeEmail, NormalizePhone, and NormalizePersonName validate and
// canonicalize the email addresses, phone numbers, and personal names people
// type. They fail with a ValidationError that never includes the input.
//
// # Civil dates
//
// Date is a calendar date with no time zone. It maps to a SQL date column and
// to YYYY-MM-DD text and JSON. NewDate, ParseDate, and DateOf build one, and
// ErrInvalidDate reports anything that is not a real calendar date from
// 0001-01-01 through 9999-12-31. Store an optional date as *Date.
//
// # Database Compatibility
//
// All components are designed for PostgreSQL (production) and SQLite (testing).
// See BaseModel documentation for type mapping details.
package models
