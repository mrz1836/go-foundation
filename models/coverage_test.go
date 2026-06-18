package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
)

// Static error fixtures used as WrapDBError inputs and hook failures. Declaring
// them at package scope keeps err113 happy (no dynamic errors.New in funcs).
var (
	errSqliteUniqueText = errors.New("UNIQUE constraint failed: widgets.name")
	errPgUniqueText     = errors.New(`duplicate key value violates unique constraint "x"`)
	errSqliteFKText     = errors.New("FOREIGN KEY constraint failed")
	errPgFKText         = errors.New(`insert violates foreign key constraint "x"`)
	errRandomText       = errors.New("some random failure")
	errHookFailed       = errors.New("hook failed")
)

func TestWrapDBError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		err     error
		wantNil bool
		target  error
	}{
		{name: "nil passes through", err: nil, wantNil: true},
		{name: "record not found maps to ErrNotFound", err: gorm.ErrRecordNotFound, target: models.ErrNotFound},
		{
			name:   "validation error preserved",
			err:    models.NewValidationError("field", "bad"),
			target: models.ErrValidation,
		},
		{
			name:   "pg unique violation",
			err:    &pgconn.PgError{Code: "23505", Message: "dup"},
			target: models.ErrDuplicateKey,
		},
		{
			name:   "pg foreign key violation",
			err:    &pgconn.PgError{Code: "23503", Message: "fk"},
			target: models.ErrForeignKey,
		},
		{
			name:   "pg other code",
			err:    &pgconn.PgError{Code: "42P01", Message: "no table"},
			target: models.ErrDatabaseError,
		},
		{
			name:   "sqlite unique violation",
			err:    sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique},
			target: models.ErrDuplicateKey,
		},
		{
			name:   "sqlite primary key violation",
			err:    sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintPrimaryKey},
			target: models.ErrDuplicateKey,
		},
		{
			name:   "sqlite foreign key violation",
			err:    sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey},
			target: models.ErrForeignKey,
		},
		{
			name:   "sqlite other constraint",
			err:    sqlite3.Error{Code: sqlite3.ErrError},
			target: models.ErrDatabaseError,
		},
		{
			name:   "string fallback unique (sqlite text)",
			err:    errSqliteUniqueText,
			target: models.ErrDuplicateKey,
		},
		{
			name:   "string fallback unique (postgres text)",
			err:    errPgUniqueText,
			target: models.ErrDuplicateKey,
		},
		{
			name:   "string fallback foreign key (sqlite text)",
			err:    errSqliteFKText,
			target: models.ErrForeignKey,
		},
		{
			name:   "string fallback foreign key (postgres text)",
			err:    errPgFKText,
			target: models.ErrForeignKey,
		},
		{
			name:   "unrecognized error maps to ErrDatabaseError",
			err:    errRandomText,
			target: models.ErrDatabaseError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := models.WrapDBError(tc.err)
			if tc.wantNil {
				assert.NoError(t, got)

				return
			}

			require.Error(t, got)
			assert.ErrorIs(t, got, tc.target)
		})
	}
}

func TestValidateOptionalUUID(t *testing.T) {
	t.Parallel()

	valid := models.NewID()
	empty := ""
	invalid := "not-a-uuid"

	cases := []struct {
		name    string
		input   *string
		wantErr bool
	}{
		{name: "nil pointer is valid", input: nil},
		{name: "empty string is valid", input: &empty},
		{name: "valid uuid", input: &valid},
		{name: "invalid uuid", input: &invalid, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := models.ValidateOptionalUUID(tc.input, "ref_id")
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, models.ErrValidation)
				assert.Contains(t, err.Error(), "ref_id")

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestHookRunner_RunAfterUpdateAndDelete(t *testing.T) {
	t.Parallel()

	t.Run("after-update runs hooks and propagates error", func(t *testing.T) {
		t.Parallel()

		var calls int

		runner := models.NewHookRunner(
			models.WithAfterUpdate(func(_ *gorm.DB, _ any) error { calls++; return nil }),
			models.WithAfterUpdate(func(_ *gorm.DB, _ any) error { calls++; return errHookFailed }),
			models.WithAfterUpdate(func(_ *gorm.DB, _ any) error { calls++; return nil }),
		)

		err := runner.RunAfterUpdate(nil, struct{}{})
		require.ErrorIs(t, err, errHookFailed)
		assert.Equal(t, 2, calls, "execution stops at the first failing hook")
	})

	t.Run("after-delete runs all hooks on success", func(t *testing.T) {
		t.Parallel()

		var calls int

		runner := models.NewHookRunner(
			models.WithAfterDelete(func(_ *gorm.DB, _ any) error { calls++; return nil }),
			models.WithAfterDelete(func(_ *gorm.DB, _ any) error { calls++; return nil }),
		)

		require.NoError(t, runner.RunAfterDelete(nil, struct{}{}))
		assert.Equal(t, 2, calls)
	})

	t.Run("after-delete propagates error", func(t *testing.T) {
		t.Parallel()

		runner := models.NewHookRunner(
			models.WithAfterDelete(func(_ *gorm.DB, _ any) error { return errHookFailed }),
		)
		require.ErrorIs(t, runner.RunAfterDelete(nil, struct{}{}), errHookFailed)
	})
}

// TestRepository_DBErrorPaths drives every repository method against a closed
// database so the result.Error → WrapDBError branch is exercised in each.
func TestRepository_DBErrorPaths(t *testing.T) {
	t.Parallel()

	db := newRepoDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	repo := models.NewRepository[widget, widgetID](db)
	ctx := context.Background()
	id := widgetID(models.NewID())

	require.Error(t, repo.Create(ctx, &widget{Name: "x"}))
	require.Error(t, repo.Update(ctx, &widget{Name: "x"}))

	_, err = repo.Find(ctx, id)
	require.Error(t, err)

	_, err = repo.FindAll(ctx)
	require.Error(t, err)

	_, err = repo.FindOne(ctx)
	require.Error(t, err)

	_, err = repo.Count(ctx)
	require.Error(t, err)

	_, err = repo.Exists(ctx, id)
	require.Error(t, err)

	require.Error(t, repo.Delete(ctx, id))
	require.Error(t, repo.Restore(ctx, id))
	require.Error(t, repo.HardDelete(ctx, id))
}
