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
// # Database Compatibility
//
// All components are designed for PostgreSQL (production) and SQLite (testing).
// See BaseModel documentation for type mapping details.
package models
