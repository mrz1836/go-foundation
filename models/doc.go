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
// WithOrderBy and WithSelect take column names (name or table.name) that the
// developer writes; anything else fails the query with an error wrapping
// ErrValidation before any SQL is built. WithCondition's query and the
// conditions given to WithPreload are SQL, so they too are written by the
// developer, with request values passed as bound arguments. To sort by a
// client's choice, read it against an allowlist of keys:
//
//	fields, err := models.ParseSort(r.URL.Query().Get("sort"), map[string]string{
//	    "created_at": "created_at",
//	    "name":       "display_name",
//	})
//	if err != nil {
//	    return err // a *ValidationError for the "sort" field
//	}
//	states, err := repo.FindAll(ctx, models.WithSort(fields))
//
// WithSort adds no tie-breaker, so rows with equal sort values come back in
// any order.
//
// # Keyset pagination
//
// WithKeyset pages a query by a pagination.Keyset position: it orders by a time
// column and the same table's id, and keeps only the rows after the position,
// so a page break never skips or repeats a row, however many rows share a
// second. Fetch one row more than a page to learn whether another page exists,
// and give the client the last row's position as an opaque cursor:
//
//	k, err := pagination.DecodeKeyset(cursor) // the first page is pagination.Keyset{}
//	events, err := repo.FindAll(ctx,
//	    models.WithKeyset("created_at", true, k),
//	    models.WithLimit(limit+1),
//	)
//	if len(events) > limit {
//	    events = events[:limit]
//	    last := events[limit-1]
//	    next = pagination.EncodeKeyset(last.CreatedAt, last.ID)
//	}
//
// A cursor from pagination.EncodeCursor decodes as a legacy position: its
// second is a lower bound, so the next page may repeat that second's rows but
// never skips one. The column is written by the developer, never taken from a
// request. On SQLite, which stores times as text, the comparison is exact only
// for rows stored in UTC; PostgreSQL compares instants.
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
// # Database Compatibility
//
// All components are designed for PostgreSQL (production) and SQLite (testing).
// See BaseModel documentation for type mapping details.
package models
