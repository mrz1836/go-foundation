package models

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// sortField is the field name ParseSort's validation errors carry.
const sortField = "sort"

// SortField is one term of a sort: the client-facing key, the column it maps
// to, and its direction.
type SortField struct {
	Key    string
	Column string
	Desc   bool
}

// ParseSort reads a client's sort, such as "-created_at,name", against an
// allowlist that maps each key a client may send to the column it sorts by.
// Keys are separated by commas, and a leading "-" sorts that key descending.
// An empty expr is no sort: nil and no error.
//
// It refuses, with a *ValidationError whose Field is "sort", a sort that holds
// any whitespace, an empty key, a key the allowlist doesn't hold, or a key that
// appears more than once in either direction. The messages name the allowed
// keys, never the input.
func ParseSort(expr string, allowed map[string]string) ([]SortField, error) {
	if expr == "" {
		return nil, nil
	}

	if strings.ContainsFunc(expr, unicode.IsSpace) {
		return nil, NewValidationError(sortField, "sort keys must not contain spaces")
	}

	segments := strings.Split(expr, ",")
	fields := make([]SortField, 0, len(segments))
	seen := make(map[string]bool, len(segments))

	for _, segment := range segments {
		field, err := parseSortKey(segment, allowed)
		if err != nil {
			return nil, err
		}

		if seen[field.Key] {
			return nil, NewValidationError(sortField, "a sort key appears more than once")
		}

		seen[field.Key] = true
		fields = append(fields, field)
	}

	return fields, nil
}

// WithSort orders a query by each field's column in turn, quoted for the
// dialect, with an explicit ASC or DESC. It adds no tie-breaker, so a list that
// pages needs a unique last column, or keyset paging (WithKeyset). No fields
// order nothing.
//
// Each column must be a name or table.name, as ParseSort's allowlist maps them;
// anything else fails the query with an error wrapping ErrValidation before any
// SQL is built.
func WithSort(fields []SortField) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		for _, f := range fields {
			if err := checkColumn(f.Column); err != nil {
				return failQuery(db, err)
			}
		}

		for _, f := range fields {
			db = orderColumn(db, f.Column, f.Desc)
		}

		return db
	}
}

// parseSortKey reads one comma-separated key of a sort: an optional "-", then
// a key the allowlist holds.
func parseSortKey(segment string, allowed map[string]string) (SortField, error) {
	key, desc := strings.CutPrefix(segment, "-")
	if key == "" {
		return SortField{}, NewValidationError(sortField, "empty sort key")
	}

	column, ok := allowed[key]
	if !ok {
		keys := slices.Sorted(maps.Keys(allowed))

		return SortField{}, NewValidationError(sortField, "unknown sort key; allowed: "+strings.Join(keys, ", "))
	}

	return SortField{Key: key, Column: column, Desc: desc}, nil
}
