package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// SQLite extended result codes for constraint violations. They are declared
// locally — rather than imported from a driver package — because these three
// values are part of the stable SQLite ABI. Keeping them here lets the matcher
// stay driver-agnostic (see sqliteCoder).
const (
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
)

// sqliteCoder is the error behavior exposed by the pure-Go SQLite drivers
// (modernc.org/sqlite and its glebarez/go-sqlite fork): the extended result code
// via Code() int. Matching the behavior rather than a concrete type keeps this
// package free of any SQLite driver import — so it builds without cgo and stays
// decoupled from the test database choice.
type sqliteCoder interface{ Code() int }

// Repository provides generic CRUD operations for models. It is doubly
// generic — over the entity type T and over the entity's typed ID — so
// per-context repositories inherit fully-typed CRUD without per-entity shim
// methods. The active database handle is read from ctx via DBFrom, so a
// repository call made inside Transactor.WithinTx automatically picks up the
// transaction.
//
// A repository may be configured with a separate read replica (see
// NewRepositoryWithReadWrite): read methods then target the replica while write
// methods target the primary. Reads issued inside an active transaction still
// use the transaction's connection (the primary), preserving read-your-writes
// consistency. When no replica is configured, reads and writes share one handle.
type Repository[T any, ID ~string] struct {
	db     *gorm.DB // primary/write connection
	readDB *gorm.DB // read connection; equals db when no replica is configured
}

// NewRepository creates a new generic repository instance bound to db. Reads and
// writes share the single connection.
func NewRepository[T any, ID ~string](db *gorm.DB) *Repository[T, ID] {
	return &Repository[T, ID]{db: db, readDB: db}
}

// NewRepositoryWithReadWrite creates a repository that routes writes to writeDB
// and reads to readDB (a read replica). Reads made inside an active transaction
// still use the transaction's connection, so they observe uncommitted writes.
// A nil readDB falls back to writeDB, making this equivalent to NewRepository.
func NewRepositoryWithReadWrite[T any, ID ~string](writeDB, readDB *gorm.DB) *Repository[T, ID] {
	if readDB == nil {
		readDB = writeDB
	}

	return &Repository[T, ID]{db: writeDB, readDB: readDB}
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

	db := ApplyOptions(DBFrom(ctx, r.readDB).WithContext(ctx), opts...)

	result := db.First(&entity, "id = ?", string(id))
	if result.Error != nil {
		return nil, WrapDBError(result.Error)
	}

	return &entity, nil
}

// FindAll retrieves all records matching the query options.
func (r *Repository[T, ID]) FindAll(ctx context.Context, opts ...QueryOption) ([]T, error) {
	var entities []T

	db := ApplyOptions(DBFrom(ctx, r.readDB).WithContext(ctx), opts...)

	result := db.Find(&entities)
	if result.Error != nil {
		return nil, WrapDBError(result.Error)
	}

	return entities, nil
}

// FindOne retrieves a single record matching the query options.
func (r *Repository[T, ID]) FindOne(ctx context.Context, opts ...QueryOption) (*T, error) {
	var entity T

	db := ApplyOptions(DBFrom(ctx, r.readDB).WithContext(ctx), opts...)

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

// execByID runs a mutating query keyed by a typed ID. It validates the id,
// invokes build against the write connection to produce the result, and maps the
// outcome to WrapDBError / ErrNotFound / nil. It centralizes the guard and
// result handling shared by Delete, Restore, and HardDelete.
func (r *Repository[T, ID]) execByID(ctx context.Context, id ID, build func(db *gorm.DB, entity *T) *gorm.DB) error {
	if err := ValidateUUID(string(id)); err != nil {
		return err
	}

	var entity T

	result := build(DBFrom(ctx, r.db).WithContext(ctx), &entity)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete soft-deletes a record by its typed ID.
func (r *Repository[T, ID]) Delete(ctx context.Context, id ID) error {
	return r.execByID(ctx, id, func(db *gorm.DB, entity *T) *gorm.DB {
		return db.Where("id = ?", string(id)).Delete(entity)
	})
}

// Restore un-deletes a soft-deleted record.
func (r *Repository[T, ID]) Restore(ctx context.Context, id ID) error {
	return r.execByID(ctx, id, func(db *gorm.DB, entity *T) *gorm.DB {
		return db.
			Session(&gorm.Session{SkipHooks: true}).
			Unscoped().Model(entity).Where("id = ?", string(id)).Update("deleted_at", nil)
	})
}

// HardDelete permanently removes a record from the database. Use with caution
// — this operation is irreversible.
func (r *Repository[T, ID]) HardDelete(ctx context.Context, id ID) error {
	return r.execByID(ctx, id, func(db *gorm.DB, entity *T) *gorm.DB {
		return db.Unscoped().Where("id = ?", string(id)).Delete(entity)
	})
}

// Count returns the number of records matching the query options.
func (r *Repository[T, ID]) Count(ctx context.Context, opts ...QueryOption) (int64, error) {
	var (
		count  int64
		entity T
	)

	db := ApplyOptions(DBFrom(ctx, r.readDB).WithContext(ctx).Model(&entity), opts...)

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

	result := DBFrom(ctx, r.readDB).WithContext(ctx).Model(&entity).Where("id = ?", string(id)).Count(&count)
	if result.Error != nil {
		return false, WrapDBError(result.Error)
	}

	return count > 0, nil
}

// DB returns the bound primary/write database handle. Repositories that need
// direct GORM access (custom joins, raw SQL) can use this. Prefer
// DBFrom(ctx, r.DB()) so that an active transaction is picked up.
func (r *Repository[T, ID]) DB() *gorm.DB {
	return r.db
}

// ReadDB returns the read database handle (the read replica when configured,
// otherwise the primary). Prefer DBFrom(ctx, r.ReadDB()) so that an active
// transaction is picked up and observes uncommitted writes.
func (r *Repository[T, ID]) ReadDB() *gorm.DB {
	return r.readDB
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

// classifyConstraintError wraps cause with the sentinel matching the detected
// constraint kind: ErrDuplicateKey for a unique/primary-key violation,
// ErrForeignKey for a foreign-key violation, and ErrDatabaseError otherwise. It
// centralizes the three-way error construction shared by every driver matcher.
//
// cause is wrapped with %w (not flattened to a string) so the original driver or
// stdlib error stays in the chain: callers can still errors.Is a
// context.DeadlineExceeded / serialization failure or errors.As the concrete
// *pgconn.PgError to decide whether a failed transaction is retryable.
func classifyConstraintError(unique, fk bool, cause error) error {
	switch {
	case unique:
		return fmt.Errorf("%w: %w", ErrDuplicateKey, cause)
	case fk:
		return fmt.Errorf("%w: %w", ErrForeignKey, cause)
	default:
		return fmt.Errorf("%w: %w", ErrDatabaseError, cause)
	}
}

// wrapPgError matches PostgreSQL (pgx) errors via SQLSTATE codes, which are
// stable across driver versions.
func wrapPgError(err error) (bool, error) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false, nil
	}

	return true, classifyConstraintError(
		pgErr.Code == pgCodeUniqueViolation,
		pgErr.Code == pgCodeForeignKeyViolation,
		err,
	)
}

// wrapSqliteError matches SQLite errors by their extended result code, exposed
// by the pure-Go driver through the sqliteCoder behavior. Because the driver is
// pure Go, this compiles and runs without cgo: the production build
// (CGO_ENABLED=0, e.g. a static Lambda) and the test build share one code path.
// SQLite is a development/test database; production uses PostgreSQL (pgx), whose
// errors are matched earlier by wrapPgError.
func wrapSqliteError(err error) (bool, error) {
	var coder sqliteCoder
	if !errors.As(err, &coder) {
		return false, nil
	}

	code := coder.Code()

	return true, classifyConstraintError(
		code == sqliteConstraintUnique || code == sqliteConstraintPrimaryKey,
		code == sqliteConstraintForeignKey,
		err,
	)
}

// wrapByMessage is a defensive fallback for unwrapped driver errors. The
// slog.Warn highlights any driver-format change so we can promote it to a
// typed match.
func wrapByMessage(err error) error {
	errStr := err.Error()
	unique := strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "duplicate key value violates unique constraint")
	fk := strings.Contains(errStr, "FOREIGN KEY constraint failed") ||
		strings.Contains(errStr, "violates foreign key constraint")

	switch {
	case unique:
		slog.Warn("models.WrapDBError: matched unique violation by string; driver error not unwrapped", "err", errStr)
	case fk:
		slog.Warn("models.WrapDBError: matched FK violation by string; driver error not unwrapped", "err", errStr)
	}

	return classifyConstraintError(unique, fk, err)
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
