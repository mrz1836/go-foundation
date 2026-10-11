package models

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mrz1836/go-foundation/pagination"
)

// WithKeyset pages a query by a keyset position: it orders by column and then
// the same table's id, both descending when desc, and keeps only the rows after
// k in that order.
//
// For a position with an id, the bound is strictly after (k.At, k.ID), with At
// bound in UTC. A legacy position (one decoded from a whole-second cursor)
// keeps the rows to the end of its second descending, or from its start
// ascending, so the next page may repeat that second's rows but never skips
// one. The zero Keyset adds only the order, so every page runs the same query
// shape. The caller adds the limit (one more than a page, to learn whether
// another page exists) and applies no other order.
//
// column is written by the developer, never taken from a request, as name or
// table.name; anything else fails the query with an error wrapping
// ErrValidation before any SQL is built. The cursor for the next page is
// pagination.EncodeKeyset of the last row's time, as the database returned it,
// and its id.
//
// On SQLite, which stores times as text, the comparison is exact only for rows
// stored in UTC (GORM's default clock is the local zone); PostgreSQL compares
// instants. An index whose leading columns are (column, id), in the list's
// order, serves the query. A position with an empty id compares the id column
// with the empty string, which an id column of a uuid type refuses, so build
// positions from rows or decode them from cursors.
func WithKeyset(column string, desc bool, k pagination.Keyset) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if err := checkColumn(column); err != nil {
			return failQuery(db, err)
		}

		db = keysetBound(db, column, desc, k)
		db = orderColumn(db, column, desc)

		return orderColumn(db, idColumnOf(column), desc)
	}
}

// checkColumn reports whether column is a plain or table-qualified name: one
// or two parts joined by ".", each an ASCII letter or underscore followed by
// letters, digits, or underscores. Its error never contains the column.
func checkColumn(column string) error {
	table, name, qualified := strings.Cut(column, ".")
	if !qualified {
		table, name = "", table
	}

	if (qualified && !isIdentifier(table)) || !isIdentifier(name) {
		return fmt.Errorf("%w: a column must be a name or table.name", ErrValidation)
	}

	return nil
}

// isIdentifier reports whether s is an ASCII letter or underscore followed by
// letters, digits, or underscores.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for i := range len(s) {
		c := s[i]
		if !isNameStart(c) && (i == 0 || !isDigit(c)) {
			return false
		}
	}

	return true
}

// isNameStart reports whether c is an ASCII letter or an underscore.
func isNameStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// failQuery records err on a fresh instance of db, so a refusal stays with its
// own query and never reaches a shared *gorm.DB. GORM sends no statement once
// a query carries an error.
func failQuery(db *gorm.DB, err error) *gorm.DB {
	tx := db.Scopes()
	_ = tx.AddError(err)

	return tx
}

// orderColumn orders by column, quoted by the dialect, with an explicit ASC or
// DESC. column must have passed checkColumn.
func orderColumn(db *gorm.DB, column string, desc bool) *gorm.DB {
	var b strings.Builder

	db.QuoteTo(&b, column)

	if desc {
		b.WriteString(" DESC")
	} else {
		b.WriteString(" ASC")
	}

	return db.Order(b.String())
}

// idColumnOf returns the id column of column's table: "id", or "t.id" for
// "t.created_at".
func idColumnOf(column string) string {
	if table, _, qualified := strings.Cut(column, "."); qualified {
		return table + ".id"
	}

	return "id"
}

// keysetBound adds the condition that keeps the rows after k.
func keysetBound(db *gorm.DB, column string, desc bool, k pagination.Keyset) *gorm.DB {
	if k.IsZero() {
		return db
	}

	if k.Legacy {
		return legacyBound(db, column, desc, k.At)
	}

	op := ">"
	if desc {
		op = "<"
	}

	return db.Where(clause.Expr{
		SQL:  "(?, ?) " + op + " (?, ?)",
		Vars: []any{clause.Column{Name: column}, clause.Column{Name: idColumnOf(column)}, k.At.UTC(), k.ID},
	})
}

// legacyBound keeps the rows from a whole-second position on: to the end of
// its second descending, from its start ascending. The second is a lower bound
// on its row's time, so the bound may repeat rows but never skips one.
func legacyBound(db *gorm.DB, column string, desc bool, at time.Time) *gorm.DB {
	second := at.UTC().Truncate(time.Second)

	if desc {
		return db.Where(clause.Lt{Column: clause.Column{Name: column}, Value: second.Add(time.Second)})
	}

	return db.Where(clause.Gte{Column: clause.Column{Name: column}, Value: second})
}
