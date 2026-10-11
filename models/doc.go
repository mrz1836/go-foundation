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
// it), EndEdge or EndEdgeAt (record its end, now or at a given time, with a
// closed copy), or SuppressEdge (take it down), inside a transaction (see
// Transactor). Each acts only on an active edge (ActiveEdgePredicate) that
// matches the id and the caller's EdgeWhere conditions, and returns
// ErrNotFound otherwise. On any error, roll the transaction back; WithinTx does
// when its callback returns the error:
//
//	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
//	    _, err := models.EndEdge[PartyPhone](ctx, edgeID, models.EdgeWhere("person_id = ?", personID))
//	    return err
//	})
//
// EndEdgeAt ends an edge at a given time instead of now, such as the date a
// source reports: the closed copy's ValidTo is that time, which may be neither
// zero, nor after the clock's now, nor before the edge's ValidFrom.
// EndEdgeAtWith also lets a callback set the closed copy's own columns:
//
//	_, err := models.EndEdgeAt[Membership](ctx, edgeID, endedAt, models.EdgeWhere("member_id = ?", memberID))
//
// The helpers above act only on an active edge. To act on an edge after it
// has ended, SupersedeCurrentEdge and SuppressCurrentEdge match its current
// row (CurrentEdgePredicate): supersede an ended edge's closed copy with an
// open copy to reopen it, or with another closed copy to correct its end, and
// suppress it to take the edge down.
//
// The superseded_by_id column must carry no immediate foreign key to its own
// table (none, or one declared DEFERRABLE INITIALLY DEFERRED), because a
// supersede points it at the row it inserts next. The VerificationStatus
// constants (VerificationStatusUnverified and the rest) are the conventional
// statuses.
//
// # Normalizers
//
// NormalizeEmail, ParsePhone, and NormalizePersonName validate and
// canonicalize the email addresses, phone numbers, and personal names people
// type. They fail with a ValidationError that never includes the input.
//
// Input is flexible and output is standard. NormalizeEmail trims, folds case,
// and removes the wrappers an address is pasted in (mailto:, enclosing angle
// brackets or quotes, a trailing comma or semicolon). It returns the Mailbox
// mail is delivered to, the canonical Address with provider domain aliases
// applied (googlemail.com is gmail.com), and the alias Root; its options
// (RejectQuotedLocal, RejectTrailingDot, RequireDottedDomain, or StrictEmail
// for all three) refuse forms the broad RFC set allows. ParsePhone drops all
// but the digits and plus signs (labels, punctuation, tel: and sms: schemes),
// validates the number against its region's numbering plan, and returns its
// E.164 and international forms; its options set a default region, require a
// number in use (RequireValidNumber), or read keypad letters (KeypadLetters):
//
//	email, err := models.NormalizeEmail(" <Jane@Example.COM> ", models.StrictEmail())
//	// email.Address == "jane@example.com"
//
//	phone, err := models.ParsePhone("Phone: (305) 555-0100",
//	    models.WithDefaultRegion("US"), models.RequireValidNumber())
//	// phone.E164 == "+13055550100", phone.International == "+1 305-555-0100"
//
// NormalizePhone, which checks only the shape of E.164, is deprecated in favor
// of ParsePhone.
//
// # Civil dates
//
// Date is a calendar date with no time zone. It maps to a SQL date column and
// to YYYY-MM-DD text and JSON. NewDate, ParseDate, and DateOf build one, and
// ErrInvalidDate reports anything that is not a real calendar date from
// 0001-01-01 through 9999-12-31. Store an optional date as *Date.
//
// # Statement timeouts
//
// WithinStatementTimeout runs a function in a transaction in which PostgreSQL
// cancels any statement that runs longer than a time bound, and reports such a
// statement with an error wrapping ErrStatementTimeout. Pass the read replica
// (Repository.ReadDB) for reads and the primary for writes; a transaction that
// ctx already carries wins:
//
//	err := models.WithinStatementTimeout(ctx, repo.ReadDB(), 2*time.Second,
//	    func(ctx context.Context, tx *gorm.DB) error {
//	        return tx.Where("name LIKE ?", prefix+"%").Find(&states).Error
//	    })
//	if errors.Is(err, models.ErrStatementTimeout) {
//	    // the search ran too long
//	}
//
// The bound is local to the transaction, so it ends when the transaction does,
// and a call nested in another transaction sets the enclosing bound back when
// it returns. On other databases there is no bound, though the function still
// runs in a transaction. Behind a connection proxy such as Amazon RDS Proxy,
// setting the bound pins the client connection to its database connection; a
// role default (ALTER ROLE ... SET statement_timeout) or the proxy's
// initialization query bounds statements and keeps the proxy's multiplexing.
//
// # Database Compatibility
//
// All components are designed for PostgreSQL (production) and SQLite (testing).
// See BaseModel documentation for type mapping details.
package models
