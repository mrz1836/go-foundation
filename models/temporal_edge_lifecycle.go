package models

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
// parentheses, with the edge id and ActiveEdgePredicate. Build it with EdgeWhere.
type EdgeCondition struct {
	query string
	args  []any
}

// EdgeWhere returns an EdgeCondition, for example EdgeWhere("owner_id = ?", ownerID).
// query is SQL written by the caller, never input: one boolean expression whose
// parentheses balance, with values passed only as args. The helpers refuse, with
// an error wrapping ErrValidation, a query that is blank, whose parentheses don't
// balance, or that holds ";", "--", or "/*", because such a fragment could escape
// the parentheses they add and widen the match.
func EdgeWhere(query string, args ...any) EdgeCondition {
	return EdgeCondition{query: query, args: args}
}

// SupersedeEdge replaces the active edge id with replacement: it points the
// edge's superseded_by_id at the replacement, then inserts the replacement.
// It is how an edge is corrected without updating it in place.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition. Before any SQL runs, it refuses a malformed id or
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
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return err
	}

	if err = prepareReplacement[T](id, replacement); err != nil {
		return err
	}

	res := matchActiveEdge(ctx, tx, string(id), conds).
		Model(new(T)).
		UpdateColumn("superseded_by_id", string(replacement.Edge().ID))
	if err = edgeUpdateResult(res); err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Omit(clause.Associations).Create(replacement).Error; err != nil {
		return WrapDBError(err)
	}

	return nil
}

// EndEdge records that the active edge id has ended, without updating it in
// place: it supersedes the edge with a closed copy and returns the copy. The
// copy has the edge's fields (valid_from, event time, provenance, and every
// model column), a new ID, and ValidTo, RecordedAt, and CreatedAt set to the
// context clock's now. The original keeps what was believed; the copy records
// that the relationship held from ValidFrom until now. The original's valid_to
// is never written.
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

	var current T

	err = matchActiveEdge(ctx, tx, string(id), conds).
		Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: no active edge matched", ErrNotFound)
	}

	if err != nil {
		return nil, WrapDBError(err)
	}

	closed := current
	c := PT(&closed).Edge()
	now := ClockFrom(ctx).Now(ctx)
	c.ID = ""
	c.ValidTo = &now
	c.RecordedAt = now
	c.CreatedAt = now

	if err = SupersedeEdge[T, ID, PT](ctx, id, PT(&closed), conds...); err != nil {
		return nil, err
	}

	return PT(&closed), nil
}

// SuppressEdge takes the active edge id down: it sets its suppressed_at to the
// context clock's now and writes no other column. verification_status is not
// touched; VerificationStatusSuppressed is an outcome a writer records on a
// new row.
//
// It runs only in the transaction ctx carries, and returns ErrNoTransaction
// otherwise. It acts only on an active edge (ActiveEdgePredicate) that matches
// id and every condition. Before any SQL runs, it refuses a malformed id with
// ErrInvalidID, and a condition that is blank, whose parentheses don't balance,
// or that holds ";", "--", or "/*", with an error wrapping ErrValidation. When
// no edge matches, it returns an error wrapping ErrNotFound and writes nothing.
//
// On any error the caller must roll back, as Transactor.WithinTx does when its
// callback returns the error. Under PostgreSQL's READ COMMITTED, of two
// concurrent calls on one edge exactly one succeeds, because the conditional
// update re-checks the predicate after waiting for the other's row lock.
func SuppressEdge[T any, ID ~string, PT EdgeModel[T, ID]](
	ctx context.Context, id ID, conds ...EdgeCondition,
) error {
	tx, err := prepareEdgeWrite(ctx, id, conds)
	if err != nil {
		return err
	}

	now := ClockFrom(ctx).Now(ctx)

	return edgeUpdateResult(matchActiveEdge(ctx, tx, string(id), conds).
		Model(new(T)).
		UpdateColumn("suppressed_at", now))
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
	if tx == nil || tx.Statement == nil {
		return nil, ErrNoTransaction
	}

	if committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok || committer == nil {
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

// matchActiveEdge starts a statement, on a fresh session of tx, that matches
// only the active edge id and only when every condition also holds. Each
// condition is parenthesized, so a balanced one can narrow the match but never
// widen it: GORM parenthesizes a fragment only when it finds " AND " or " OR "
// with spaces around it.
func matchActiveEdge(ctx context.Context, tx *gorm.DB, id string, conds []EdgeCondition) *gorm.DB {
	stmt := tx.WithContext(ctx).Where("id = ?", id).Where(ActiveEdgePredicate)
	for _, c := range conds {
		stmt = stmt.Where("("+c.query+")", c.args...)
	}

	return stmt
}

// edgeUpdateResult maps a conditional update's result: an error through
// WrapDBError, and no matched row to an error wrapping ErrNotFound.
func edgeUpdateResult(res *gorm.DB) error {
	if res.Error != nil {
		return WrapDBError(res.Error)
	}

	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: no active edge matched", ErrNotFound)
	}

	return nil
}
