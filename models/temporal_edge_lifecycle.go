package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EdgeModel is satisfied by a pointer to any model that embeds TemporalEdge[ID].
// The lifecycle helpers take it so the edge's ID type is checked at compile
// time.
type EdgeModel[T any, ID ~string] interface {
	*T
	Edge() *TemporalEdge[ID]
}

// EdgeCondition is an extra WHERE fragment that a lifecycle helper ANDs, in
// parentheses, with the edge id and the helper's predicate (ActiveEdgePredicate,
// or CurrentEdgePredicate for the current-row helpers), so a condition can only
// narrow the match. Build it with EdgeWhere.
type EdgeCondition struct {
	query string
	args  []any
}

// EdgeWhere returns an EdgeCondition, for example EdgeWhere("owner_id = ?", ownerID).
// query is SQL written by the caller, never input: one boolean expression whose
// parentheses balance, with values passed only as args. A helper ANDs it, in
// parentheses, with the edge id and the helper's predicate (ActiveEdgePredicate,
// or CurrentEdgePredicate for the current-row helpers), so it can only narrow
// the match. The helpers refuse, with an error wrapping ErrValidation, a query
// that is blank, whose parentheses don't balance, or that holds ";", "--", or
// "/*", because such a fragment could escape the parentheses they add and widen
// the match.
func EdgeWhere(query string, args ...any) EdgeCondition {
	return EdgeCondition{query: query, args: args}
}

// SupersedeEdge replaces the active edge id with replacement: it points the
// edge's superseded_by_id at the replacement, then inserts the replacement.
// It is how an edge is corrected without updating it in place.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition; SupersedeCurrentEdge and SuppressCurrentEdge act on
// an edge after it has ended. Before any SQL runs, it refuses a malformed id or
// replacement ID with ErrInvalidID, and a condition that is blank, whose
// parentheses don't balance, or that holds ";", "--", or "/*", with an error
// wrapping ErrValidation; it refuses a nil replacement, one already superseded
// or suppressed, and one whose ID is id the same way. When no edge matches, it
// returns an error wrapping ErrNotFound and inserts nothing.
//
// The replacement's ID is minted before the update (a preset one is kept), and
// its BeforeCreate hook runs on insert; associations are not saved. Because the
// update points superseded_by_id at the row inserted next, that column must
// carry no immediate foreign key to its own table: none, or one declared
// DEFERRABLE INITIALLY DEFERRED.
//
// On any error the caller must roll back, as Transactor.WithinTx does when its
// callback returns the error: a failed insert follows a successful update.
// Under PostgreSQL's READ COMMITTED, of two concurrent calls on one edge
// exactly one succeeds, because the conditional update re-checks the predicate
// after waiting for the other's row lock.
func SupersedeEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, replacement PT, conds ...EdgeCondition,
) error {
	return supersedeEdge[T, ID, PT](ctx, id, replacement, ActiveEdgePredicate, conds)
}

// EndEdge records that the active edge id has ended now, without updating it
// in place: it is EndEdgeAt with the context clock's now as the end, so the
// closed copy's ValidTo, RecordedAt, and CreatedAt are one instant. It
// supersedes the edge with the copy and returns the copy. The copy has the
// edge's fields (valid_from, event time, provenance, and every model column)
// and a new ID. The original keeps what was believed; the copy records that
// the relationship held from ValidFrom until now. The original's valid_to is
// never written. Unlike EndEdgeAt, it never compares the end with the edge's
// ValidFrom, so an edge whose ValidFrom is later than now still ends, at now.
//
// It locks the active row as it reads it (FOR UPDATE where the database has
// row locks), so no other transaction changes the row between that read and
// the supersede, and the copy holds the row's committed values. A lost race is
// still no match: after waiting for the lock, PostgreSQL re-checks the
// predicate.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition. Before any SQL runs, it refuses a malformed id with
// ErrInvalidID, and a condition that is blank, whose parentheses don't balance,
// or that holds ";", "--", or "/*", with an error wrapping ErrValidation. When
// no edge matches, it returns an error wrapping ErrNotFound and writes nothing.
//
// As with SupersedeEdge, superseded_by_id must carry no immediate foreign key
// to its own table (none, or one declared DEFERRABLE INITIALLY DEFERRED), and
// on any error the caller must roll back. Under PostgreSQL's READ COMMITTED,
// of two concurrent calls on one edge exactly one succeeds.
func EndEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, conds ...EdgeCondition,
) (PT, error) {
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return nil, err
	}

	now := ClockFrom(ctx).Now(ctx)

	return closeEdge[T, ID, PT](ctx, tx, id, edgeEnd{at: now, now: now}, nil, conds)
}

// EndEdgeAt records that the active edge id ended at validTo, without updating
// it in place: it supersedes the edge with a closed copy and returns the copy.
// Use it when the end is a known time, such as one a source reports; EndEdge
// ends an edge at the clock's now. The copy has the edge's fields, a new ID,
// ValidTo set to validTo as given, and RecordedAt and CreatedAt set to the
// context clock's now, read once before any SQL runs. The original's valid_to
// is never written.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition. Before any SQL runs, it refuses a malformed id with
// ErrInvalidID; and, with an error wrapping ErrValidation, a condition that is
// blank, whose parentheses don't balance, or that holds ";", "--", or "/*", and
// a validTo that is zero or after the clock's now. After the read, it returns
// an error wrapping ErrNotFound when no edge matches, and an error wrapping
// ErrValidation when validTo is before the edge's ValidFrom; an end equal to
// ValidFrom is allowed. A refused call writes nothing.
//
// The time refusals are programming errors, not a *ValidationError: a caller
// that takes the time from a person checks it first and reports its own field
// error.
//
// Like EndEdge, it locks the active row as it reads it, so the copy holds the
// row's committed values; superseded_by_id must carry no immediate foreign key
// to its own table (none, or one declared DEFERRABLE INITIALLY DEFERRED); on
// any error the caller must roll back; and under PostgreSQL's READ COMMITTED,
// of two concurrent calls on one edge exactly one succeeds. It is
// EndEdgeAtWith with no finish.
func EndEdgeAt[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, validTo time.Time, conds ...EdgeCondition,
) (PT, error) {
	return EndEdgeAtWith[T, ID, PT](ctx, id, validTo, nil, conds...)
}

// EndEdgeAtWith is EndEdgeAt with one more step, for a closed copy that carries
// columns of its own, such as why the edge ended or what ended it. After the
// read and the start check, and before anything is written, it calls finish
// once with the copy, while the edge's row is locked. It then sets the copy's
// ID, ValidTo, RecordedAt, and CreatedAt itself, so finish can't change them.
// A copy that finish leaves superseded or suppressed is refused with an error
// wrapping ErrValidation, and nothing is written. A call that EndEdgeAt would
// refuse never calls finish. finish should only set fields; a nil finish is
// EndEdgeAt.
func EndEdgeAtWith[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, validTo time.Time, finish func(closed PT), conds ...EdgeCondition,
) (PT, error) {
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return nil, err
	}

	now := ClockFrom(ctx).Now(ctx)
	if err = checkEdgeEnd(validTo, now); err != nil {
		return nil, err
	}

	return closeEdge[T, ID, PT](ctx, tx, id, edgeEnd{at: validTo, now: now, checkStart: true}, finish, conds)
}

// SuppressEdge takes the active edge id down: it sets its suppressed_at to the
// context clock's now and writes no other column. verification_status is not
// touched; VerificationStatusSuppressed is an outcome a writer records on a
// new row.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition; SupersedeCurrentEdge and SuppressCurrentEdge act on
// an edge after it has ended. Before any SQL runs, it refuses a malformed id
// with ErrInvalidID, and a condition that is blank, whose parentheses don't
// balance, or that holds ";", "--", or "/*", with an error wrapping
// ErrValidation. When no edge matches, it returns an error wrapping
// ErrNotFound and writes nothing.
//
// On any error the caller must roll back, as Transactor.WithinTx does when its
// callback returns the error. Under PostgreSQL's READ COMMITTED, of two
// concurrent calls on one edge exactly one succeeds, because the conditional
// update re-checks the predicate after waiting for the other's row lock.
func SuppressEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, conds ...EdgeCondition,
) error {
	return suppressEdge[T, ID, PT](ctx, id, ActiveEdgePredicate, conds)
}

// SupersedeCurrentEdge is SupersedeEdge for the edge's current row
// (CurrentEdgePredicate) instead of its active row, so it also acts on an edge
// after it has ended: supersede an ended edge's closed copy with an open copy
// (ValidTo nil) to reopen it, or with another closed copy to correct its end.
// It checks the replacement as SupersedeEdge does, not its times.
//
// A reopened edge is active again, so when another active edge already holds
// its key under a partial unique index over active rows, the insert fails with
// an error wrapping ErrDuplicateKey, and the caller must roll back. Everything
// else (the transaction, the checks, update then insert, no match, the
// foreign-key note, and concurrency) is SupersedeEdge's.
func SupersedeCurrentEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, replacement PT, conds ...EdgeCondition,
) error {
	return supersedeEdge[T, ID, PT](ctx, id, replacement, CurrentEdgePredicate, conds)
}

// SuppressCurrentEdge is SuppressEdge for the edge's current row
// (CurrentEdgePredicate), so it takes an edge down whether or not it has ended.
// It writes only suppressed_at. Everything else (the transaction, the checks,
// no match, and concurrency) is SuppressEdge's.
func SuppressCurrentEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, conds ...EdgeCondition,
) error {
	return suppressEdge[T, ID, PT](ctx, id, CurrentEdgePredicate, conds)
}

// supersedeEdge points the superseded_by_id of the edge id's row that
// predicate selects at replacement, then inserts replacement.
func supersedeEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, replacement PT, predicate string, conds []EdgeCondition,
) error {
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return err
	}

	if err = prepareReplacement[T](id, replacement); err != nil {
		return err
	}

	res := matchEdge(ctx, tx, string(id), predicate, conds).
		Model(new(T)).
		UpdateColumn("superseded_by_id", string(replacement.Edge().ID))
	if err = edgeUpdateResult(res, predicate); err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Omit(clause.Associations).Create(replacement).Error; err != nil {
		return WrapDBError(err)
	}

	return nil
}

// suppressEdge sets suppressed_at, to the context clock's now, on the edge id's
// row that predicate selects.
func suppressEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, predicate string, conds []EdgeCondition,
) error {
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return err
	}

	now := ClockFrom(ctx).Now(ctx)

	return edgeUpdateResult(matchEdge(ctx, tx, string(id), predicate, conds).
		Model(new(T)).
		UpdateColumn("suppressed_at", now), predicate)
}

// edgeEnd is how closeEdge ends an edge: the closed copy's ValidTo (at), the
// clock reading that stamps its RecordedAt and CreatedAt (now), and whether an
// end before the edge's ValidFrom is refused (checkStart).
type edgeEnd struct {
	at         time.Time
	now        time.Time
	checkStart bool
}

// closeEdge reads and locks the active edge id, refuses an end before its
// ValidFrom when end asks, lets finish set the closed copy's own columns, and
// supersedes the edge with the copy.
func closeEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, tx *gorm.DB, id ID, end edgeEnd, finish func(closed PT), conds []EdgeCondition,
) (PT, error) {
	var current T

	err := matchEdge(ctx, tx, string(id), ActiveEdgePredicate, conds).
		Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: no active edge matched", ErrNotFound)
	}

	if err != nil {
		return nil, WrapDBError(err)
	}

	if end.checkStart && end.at.Before(PT(&current).Edge().ValidFrom) {
		return nil, fmt.Errorf("%w: the end time is before the edge's valid_from", ErrValidation)
	}

	closed := current
	if finish != nil {
		finish(PT(&closed))
	}

	c := PT(&closed).Edge()
	at := end.at
	c.ID = ""
	c.ValidTo = &at
	c.RecordedAt = end.now
	c.CreatedAt = end.now

	if err = SupersedeEdge[T, ID, PT](ctx, id, PT(&closed), conds...); err != nil {
		return nil, err
	}

	return PT(&closed), nil
}

// checkEdgeEnd refuses an end time that is zero or after now.
func checkEdgeEnd(at, now time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("%w: the end time is zero", ErrValidation)
	}

	if at.After(now) {
		return fmt.Errorf("%w: the end time is after the clock's now", ErrValidation)
	}

	return nil
}

// prepareEdgeWrite makes the checks every lifecycle helper makes before any
// SQL runs, and returns the transaction ctx carries.
func prepareEdgeWrite[ID ~string](ctx context.Context, id ID, conds []EdgeCondition) (*gorm.DB, error) {
	tx, err := edgeTx(ctx)
	if err != nil {
		return nil, err
	}

	if err = ValidateUUID(string(id)); err != nil {
		return nil, err
	}

	if err = checkEdgeConditions(conds); err != nil {
		return nil, err
	}

	return tx, nil
}

// edgeTx returns the transaction ctx carries. A context with no handle, or
// with a handle whose connection is not a transaction, is ErrNoTransaction:
// GORM itself tells an open transaction by its connection.
func edgeTx(ctx context.Context) (*gorm.DB, error) {
	tx := DBFrom(ctx, nil)
	if !inTransaction(tx) {
		return nil, ErrNoTransaction
	}

	return tx, nil
}

// checkEdgeConditions refuses a condition that is blank, or that could escape
// the parentheses the helpers add around it.
func checkEdgeConditions(conds []EdgeCondition) error {
	for i, c := range conds {
		if strings.TrimSpace(c.query) == "" {
			return fmt.Errorf("%w: edge condition %d is empty", ErrValidation, i)
		}

		if !balancedEdgeCondition(c.query) {
			return fmt.Errorf(
				"%w: edge condition %d must be one balanced expression with no comment or statement separator",
				ErrValidation, i,
			)
		}
	}

	return nil
}

// balancedEdgeCondition reports whether query holds no ";", "--", or "/*", and
// its parentheses balance: no ")" closes a "(" the query did not open, and no
// "(" is left open. Either flaw would let the query close the parentheses the
// helpers add early and widen the match; a comment could hide a parenthesis
// from the count. It counts every parenthesis, even one inside a quoted
// literal: values travel as args, so a condition needs none. It catches
// mistakes; it is not a SQL parser.
func balancedEdgeCondition(query string) bool {
	if strings.Contains(query, ";") || strings.Contains(query, "--") || strings.Contains(query, "/*") {
		return false
	}

	depth := 0

	for _, r := range query {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}

	return depth == 0
}

// prepareReplacement refuses a replacement that cannot supersede the edge id,
// and mints its ID (a preset one is kept) so the update can point at it before
// it is inserted.
func prepareReplacement[T any, ID ~string, PT EdgeModel[T, ID]](id ID, replacement PT) error {
	if replacement == nil {
		return fmt.Errorf("%w: replacement edge is nil", ErrValidation)
	}

	r := replacement.Edge()
	if r.SupersededByID != nil || r.SuppressedAt != nil {
		return fmt.Errorf("%w: a replacement edge must not be superseded or suppressed", ErrValidation)
	}

	r.ID = mintID(r.ID)
	if err := ValidateUUID(string(r.ID)); err != nil {
		return err
	}

	if r.ID == id {
		return fmt.Errorf("%w: an edge cannot supersede itself", ErrValidation)
	}

	return nil
}

// matchEdge starts a statement, on a fresh session of tx, that matches the edge
// id only when predicate (ActiveEdgePredicate or CurrentEdgePredicate) and
// every condition also hold. Each condition is parenthesized, so a balanced one
// can narrow the match but never widen it: GORM parenthesizes a fragment only
// when it finds " AND " or " OR " with spaces around it.
func matchEdge(ctx context.Context, tx *gorm.DB, id, predicate string, conds []EdgeCondition) *gorm.DB {
	stmt := tx.WithContext(ctx).Where("id = ?", id).Where(predicate)
	for _, c := range conds {
		stmt = stmt.Where("("+c.query+")", c.args...)
	}

	return stmt
}

// matchName names the rows predicate selects, for an error message: "current"
// for CurrentEdgePredicate, and "active" otherwise.
func matchName(predicate string) string {
	if predicate == CurrentEdgePredicate {
		return "current"
	}

	return "active"
}

// edgeUpdateResult maps a conditional update's result: an error through
// WrapDBError, and no matched row to an error wrapping ErrNotFound that names
// the rows predicate selects.
func edgeUpdateResult(res *gorm.DB, predicate string) error {
	if res.Error != nil {
		return WrapDBError(res.Error)
	}

	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: no %s edge matched", ErrNotFound, matchName(predicate))
	}

	return nil
}
