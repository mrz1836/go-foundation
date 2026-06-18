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
// Example:
//
//	states, err := repo.FindAll(ctx, WithSelect("id", "name"))
func WithSelect(fields ...string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if len(fields) > 0 {
			return db.Select(fields)
		}

		return db
	}
}

// WithOrderBy adds an ordering clause to the query.
// The desc parameter determines if the order is descending (true) or ascending (false).
//
// Example:
//
//	states, err := repo.FindAll(ctx, WithOrderBy("name", false)) // ORDER BY name ASC
//	cities, err := repo.FindAll(ctx, WithOrderBy("created_at", true)) // ORDER BY created_at DESC
func WithOrderBy(field string, desc bool) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if field == "" {
			return db
		}

		order := field
		if desc {
			order += " DESC"
		} else {
			order += " ASC"
		}

		return db.Order(order)
	}
}

// WithCondition adds a custom WHERE condition to the query.
// This allows for arbitrary filtering without predefined methods.
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
