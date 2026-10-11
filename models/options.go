package models

import "gorm.io/gorm"

// QueryOption is a functional option for customizing database queries.
// Options are applied in order when building queries.
type QueryOption func(*gorm.DB) *gorm.DB

// WithLimit sets the maximum number of records to return.
func WithLimit(limit int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if limit > 0 {
			return db.Limit(limit)
		}

		return db
	}
}

// WithOffset sets the number of records to skip before returning results.
func WithOffset(offset int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if offset > 0 {
			return db.Offset(offset)
		}

		return db
	}
}

// WithIncludeDeleted includes soft-deleted records in the query results.
func WithIncludeDeleted(include bool) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if include {
			return db.Unscoped()
		}

		return db
	}
}

// WithPreload eagerly loads the specified association.
// Multiple WithPreload options can be chained to load multiple associations.
//
// The association is resolved against the model's associations, but args are
// passed to GORM's Preload as they are: a condition string among them is SQL,
// like WithCondition's query, so it is written by the developer and never
// taken from a request. Pass request values as the arguments of its
// placeholders, which are bound as parameters.
//
// Example:
//
//	cities, err := repo.FindAll(ctx, WithPreload("State"), WithPreload("Neighborhoods"))
func WithPreload(association string, args ...any) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(association, args...)
	}
}

// WithSelect specifies which fields to retrieve.
// By default, all fields are selected.
//
// Each field must be a column name or table.name: one or two parts joined by
// ".", each an ASCII letter or underscore followed by letters, digits, or
// underscores. Anything else fails the query with an error wrapping
// ErrValidation before any SQL is built. Fields are written by the developer,
// never taken from a request. To select an expression, such as a count or an
// alias, write a QueryOption of your own, which states its SQL in plain sight:
//
//	count := func(db *gorm.DB) *gorm.DB { return db.Select("count(*) AS n") }
//
// Example:
//
//	states, err := repo.FindAll(ctx, WithSelect("id", "name"))
func WithSelect(fields ...string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if len(fields) == 0 {
			return db
		}

		for _, field := range fields {
			if err := checkColumn(field); err != nil {
				return failQuery(db, err)
			}
		}

		return db.Select(fields)
	}
}

// WithOrderBy adds an ordering clause to the query.
// The desc parameter determines if the order is descending (true) or ascending (false).
//
// field must be a column name or table.name, as WithSelect's fields are. It is
// quoted for the dialect, so on PostgreSQL it must match the column's case, and
// anything else fails the query with an error wrapping ErrValidation before any
// SQL is built. The column is written by the developer, never taken from a
// request: to sort by a client's choice, use ParseSort and WithSort. An empty
// field orders nothing.
//
// Example:
//
//	states, err := repo.FindAll(ctx, WithOrderBy("name", false)) // ORDER BY "name" ASC
//	cities, err := repo.FindAll(ctx, WithOrderBy("created_at", true)) // ORDER BY "created_at" DESC
func WithOrderBy(field string, desc bool) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if field == "" {
			return db
		}

		if err := checkColumn(field); err != nil {
			return failQuery(db, err)
		}

		return orderColumn(db, field, desc)
	}
}

// WithCondition adds a custom WHERE condition to the query.
// This allows for arbitrary filtering without predefined methods.
//
// query is SQL, written into WHERE as it is given, so it is written by the
// developer and never taken from a request. Pass request values through args:
// they fill the query's placeholders and are bound as parameters.
//
// Example:
//
//	cities, err := repo.FindAll(ctx, WithCondition("population > ?", 1000000))
func WithCondition(query any, args ...any) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(query, args...)
	}
}

// ApplyOptions applies all query options to the database connection.
func ApplyOptions(db *gorm.DB, opts ...QueryOption) *gorm.DB {
	for _, opt := range opts {
		db = opt(db)
	}

	return db
}
