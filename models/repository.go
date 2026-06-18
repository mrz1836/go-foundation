package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// Repository provides generic CRUD operations for models. It is doubly
// generic — over the entity type T and over the entity's typed ID — so
// per-context repositories inherit fully-typed CRUD without per-entity shim
// methods. The active database handle is read from ctx via DBFrom, so a
// repository call made inside Transactor.WithinTx automatically picks up the
// transaction.
type Repository[T any, ID ~string] struct {
	db *gorm.DB
}

// NewRepository creates a new generic repository instance bound to db.
func NewRepository[T any, ID ~string](db *gorm.DB) *Repository[T, ID] {
	return &Repository[T, ID]{db: db}
}

// Create inserts a new record into the database.
func (r *Repository[T, ID]) Create(ctx context.Context, entity *T) error {
	result := DBFrom(ctx, r.db).WithContext(ctx).Create(entity)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	return nil
}

// Find retrieves a record by its typed UUID. Validates the ID format.
func (r *Repository[T, ID]) Find(ctx context.Context, id ID, opts ...QueryOption) (*T, error) {
	if err := ValidateUUID(string(id)); err != nil {
		return nil, err
	}

	var entity T

	db := ApplyOptions(DBFrom(ctx, r.db).WithContext(ctx), opts...)

	result := db.First(&entity, "id = ?", string(id))
	if result.Error != nil {
		return nil, WrapDBError(result.Error)
	}

	return &entity, nil
}

// FindAll retrieves all records matching the query options.
func (r *Repository[T, ID]) FindAll(ctx context.Context, opts ...QueryOption) ([]T, error) {
	var entities []T

	db := ApplyOptions(DBFrom(ctx, r.db).WithContext(ctx), opts...)

	result := db.Find(&entities)
	if result.Error != nil {
		return nil, WrapDBError(result.Error)
	}

	return entities, nil
}

// FindOne retrieves a single record matching the query options.
func (r *Repository[T, ID]) FindOne(ctx context.Context, opts ...QueryOption) (*T, error) {
	var entity T

	db := ApplyOptions(DBFrom(ctx, r.db).WithContext(ctx), opts...)

	result := db.First(&entity)
	if result.Error != nil {
		return nil, WrapDBError(result.Error)
	}

	return &entity, nil
}

// Update saves changes to an existing record.
func (r *Repository[T, ID]) Update(ctx context.Context, entity *T) error {
	result := DBFrom(ctx, r.db).WithContext(ctx).Save(entity)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	return nil
}

// Delete soft-deletes a record by its typed ID.
func (r *Repository[T, ID]) Delete(ctx context.Context, id ID) error {
	if err := ValidateUUID(string(id)); err != nil {
		return err
	}

	var entity T

	result := DBFrom(ctx, r.db).WithContext(ctx).Where("id = ?", string(id)).Delete(&entity)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Restore un-deletes a soft-deleted record.
func (r *Repository[T, ID]) Restore(ctx context.Context, id ID) error {
	if err := ValidateUUID(string(id)); err != nil {
		return err
	}

	var entity T

	result := DBFrom(ctx, r.db).WithContext(ctx).
		Session(&gorm.Session{SkipHooks: true}).
		Unscoped().Model(&entity).Where("id = ?", string(id)).Update("deleted_at", nil)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// HardDelete permanently removes a record from the database. Use with caution
// — this operation is irreversible.
func (r *Repository[T, ID]) HardDelete(ctx context.Context, id ID) error {
	if err := ValidateUUID(string(id)); err != nil {
		return err
	}

	var entity T

	result := DBFrom(ctx, r.db).WithContext(ctx).Unscoped().Where("id = ?", string(id)).Delete(&entity)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Count returns the number of records matching the query options.
func (r *Repository[T, ID]) Count(ctx context.Context, opts ...QueryOption) (int64, error) {
	var (
		count  int64
		entity T
	)

	db := ApplyOptions(DBFrom(ctx, r.db).WithContext(ctx).Model(&entity), opts...)

	result := db.Count(&count)
	if result.Error != nil {
		return 0, WrapDBError(result.Error)
	}

	return count, nil
}

// Exists checks if a record with the given ID exists.
func (r *Repository[T, ID]) Exists(ctx context.Context, id ID) (bool, error) {
	if err := ValidateUUID(string(id)); err != nil {
		return false, err
	}

	var (
		count  int64
		entity T
	)

	result := DBFrom(ctx, r.db).WithContext(ctx).Model(&entity).Where("id = ?", string(id)).Count(&count)
	if result.Error != nil {
		return false, WrapDBError(result.Error)
	}

	return count > 0, nil
}

// DB returns the bound database handle. Repositories that need direct GORM
// access (custom joins, raw SQL) can use this. Prefer DBFrom(ctx, r.DB()) so
// that an active transaction is picked up.
func (r *Repository[T, ID]) DB() *gorm.DB {
	return r.db
}

// PostgreSQL SQLSTATE codes (see https://www.postgresql.org/docs/current/errcodes-appendix.html).
const (
	pgCodeUniqueViolation     = "23505"
	pgCodeForeignKeyViolation = "23503"
)

// WrapDBError converts GORM and driver errors to domain errors. It uses
// typed `errors.As` matching for both the PostgreSQL (pgx) and SQLite drivers,
// falling back to a string-based match only as a defensive last resort —
// any string fallback emits a slog.Warn so we can detect when a driver
// upgrade changes error shapes.
func WrapDBError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}

	if ok, wrapped := wrapValidationError(err); ok {
		return wrapped
	}

	if ok, wrapped := wrapPgError(err); ok {
		return wrapped
	}

	if ok, wrapped := wrapSqliteError(err); ok {
		return wrapped
	}

	return wrapByMessage(err)
}

// wrapValidationError returns validation errors unchanged so callers can
// type-assert them.
func wrapValidationError(err error) (bool, error) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return true, err
	}

	if errors.Is(err, ErrValidation) {
		return true, err
	}

	return false, nil
}

// wrapPgError matches PostgreSQL (pgx) errors via SQLSTATE codes, which are
// stable across driver versions.
func wrapPgError(err error) (bool, error) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false, nil
	}

	switch pgErr.Code {
	case pgCodeUniqueViolation:
		return true, fmt.Errorf("%w: %s", ErrDuplicateKey, pgErr.Message)
	case pgCodeForeignKeyViolation:
		return true, fmt.Errorf("%w: %s", ErrForeignKey, pgErr.Message)
	}

	return true, fmt.Errorf("%w: %s", ErrDatabaseError, pgErr.Message)
}

// wrapSqliteError matches SQLite errors via mattn/go-sqlite3 extended codes.
func wrapSqliteError(err error) (bool, error) {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false, nil
	}

	switch sqliteErr.ExtendedCode {
	case sqlite3.ErrConstraintUnique, sqlite3.ErrConstraintPrimaryKey:
		return true, fmt.Errorf("%w: %s", ErrDuplicateKey, sqliteErr.Error())
	case sqlite3.ErrConstraintForeignKey:
		return true, fmt.Errorf("%w: %s", ErrForeignKey, sqliteErr.Error())
	}

	return true, fmt.Errorf("%w: %s", ErrDatabaseError, sqliteErr.Error())
}

// wrapByMessage is a defensive fallback for unwrapped driver errors. The
// slog.Warn highlights any driver-format change so we can promote it to a
// typed match.
func wrapByMessage(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "duplicate key value violates unique constraint") {
		slog.Warn("models.WrapDBError: matched unique violation by string; driver error not unwrapped", "err", errStr)
		return fmt.Errorf("%w: %s", ErrDuplicateKey, errStr)
	}

	if strings.Contains(errStr, "FOREIGN KEY constraint failed") ||
		strings.Contains(errStr, "violates foreign key constraint") {
		slog.Warn("models.WrapDBError: matched FK violation by string; driver error not unwrapped", "err", errStr)
		return fmt.Errorf("%w: %s", ErrForeignKey, errStr)
	}

	return fmt.Errorf("%w: %s", ErrDatabaseError, errStr)
}

// ValidateUUID checks if the given string is a valid UUID.
func ValidateUUID(id string) error {
	if id == "" {
		return ErrInvalidID
	}

	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidID
	}

	return nil
}
