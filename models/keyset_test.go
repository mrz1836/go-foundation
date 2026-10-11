package models_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/pagination"
)

// keysetRow is a list row ordered by its creation time, then its id.
type keysetRow struct {
	ID        string    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
	Label     string
}

// statementRecorder is a GORM logger that keeps the SQL of every statement
// GORM traces, so a test can prove that a refused query sent nothing.
type statementRecorder struct {
	mu   sync.Mutex
	sqls []string
}

// LogMode returns the recorder itself; it records at every level.
func (r *statementRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

// Info discards the message.
func (r *statementRecorder) Info(context.Context, string, ...any) {}

// Warn discards the message.
func (r *statementRecorder) Warn(context.Context, string, ...any) {}

// Error discards the message.
func (r *statementRecorder) Error(context.Context, string, ...any) {}

// Trace records the statement's SQL.
func (r *statementRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sqls = append(r.sqls, sql)
}

// statements returns the SQL recorded so far.
func (r *statementRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.sqls)
}

// newKeysetDB opens an in-memory SQLite database on one connection, with
// keysetRow migrated.
func newKeysetDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open sqlite test db")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&keysetRow{}), "migrate keyset rows")

	return db
}

// seedKeysetRows inserts rows, every time in UTC: SQLite compares times as
// text, so a comparison is exact only between values written at one offset.
func seedKeysetRows(t *testing.T, db *gorm.DB, rows []keysetRow) {
	t.Helper()

	for i := range rows {
		rows[i].CreatedAt = rows[i].CreatedAt.UTC()
	}

	require.NoError(t, db.Create(&rows).Error)
}

// keysetOrder returns the rows' ids in (created_at, id) order.
func keysetOrder(rows []keysetRow, desc bool) []string {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b keysetRow) int {
		c := a.CreatedAt.Compare(b.CreatedAt)
		if c == 0 {
			c = strings.Compare(a.ID, b.ID)
		}

		if desc {
			return -c
		}

		return c
	})

	ids := make([]string, 0, len(sorted))
	for _, r := range sorted {
		ids = append(ids, r.ID)
	}

	return ids
}

// keysetPage reads one page after k: size rows, plus whether another follows.
func keysetPage(t *testing.T, db *gorm.DB, desc bool, k pagination.Keyset, size int) ([]keysetRow, bool) {
	t.Helper()

	var rows []keysetRow
	require.NoError(t, models.ApplyOptions(
		db.Model(&keysetRow{}),
		models.WithKeyset("created_at", desc, k),
	).Limit(size+1).Find(&rows).Error)

	if len(rows) > size {
		return rows[:size], true
	}

	return rows, false
}

// pageAll reads every page as a client would: each next position travels as
// an opaque cursor string, and the pages end when one has no extra row.
func pageAll(t *testing.T, db *gorm.DB, desc bool, size int) []string {
	t.Helper()

	var (
		ids []string
		k   pagination.Keyset
	)

	for range 100 { // a bound, so a paging bug fails instead of looping
		rows, more := keysetPage(t, db, desc, k, size)
		for _, r := range rows {
			ids = append(ids, r.ID)
		}

		if !more {
			return ids
		}

		last := rows[len(rows)-1]

		next, err := pagination.DecodeKeyset(pagination.EncodeKeyset(last.CreatedAt, last.ID))
		require.NoError(t, err)

		k = next
	}

	require.FailNow(t, "paging did not end")

	return nil
}

// sameSecondRows returns four rows inside 12:00:00 and one on each side of it,
// with ids out of time order.
func sameSecondRows() []keysetRow {
	at := func(sec, ms int) time.Time {
		return time.Date(2026, 3, 1, 12, 0, sec, ms*int(time.Millisecond), time.UTC)
	}

	return []keysetRow{
		{ID: "row-d", CreatedAt: at(0, 100)},
		{ID: "row-b", CreatedAt: at(0, 200)},
		{ID: "row-f", CreatedAt: at(0, 300)},
		{ID: "row-a", CreatedAt: at(0, 400)},
		{ID: "row-e", CreatedAt: at(-1, 900)},
		{ID: "row-c", CreatedAt: at(1, 0)},
	}
}

func TestWithKeyset_PagesRowsSharingASecond(t *testing.T) {
	t.Parallel()

	db := newKeysetDB(t)
	rows := sameSecondRows()
	seedKeysetRows(t, db, rows)

	for _, desc := range []bool{true, false} {
		for _, size := range []int{1, 2, 3} {
			assert.Equal(t, keysetOrder(rows, desc), pageAll(t, db, desc, size),
				"desc=%v size=%d: every row exactly once, in order", desc, size)
		}
	}
}

func TestWithKeyset_PagesOneTimestampByID(t *testing.T) {
	t.Parallel()

	db := newKeysetDB(t)
	at := time.Date(2026, 3, 1, 12, 0, 0, 250_000_000, time.UTC)

	ids := []string{"row-g", "row-c", "row-a", "row-e", "row-b", "row-f", "row-d"}

	rows := make([]keysetRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, keysetRow{ID: id, CreatedAt: at})
	}

	seedKeysetRows(t, db, rows)

	for _, desc := range []bool{true, false} {
		assert.Equal(t, keysetOrder(rows, desc), pageAll(t, db, desc, 2),
			"desc=%v: rows at one instant page by id, each once", desc)
	}
}

func TestWithKeyset_LegacyCursorRepeatsButNeverSkips(t *testing.T) {
	t.Parallel()

	db := newKeysetDB(t)
	rows := sameSecondRows()[:4] // the four rows inside 12:00:00
	seedKeysetRows(t, db, rows)

	all := keysetOrder(rows, true)

	t.Run("descending", func(t *testing.T) {
		first, more := keysetPage(t, db, true, pagination.Keyset{}, 2)
		require.True(t, more)

		last := first[len(first)-1]
		legacy, err := pagination.DecodeKeyset(pagination.EncodeCursor(last.CreatedAt))
		require.NoError(t, err)
		require.True(t, legacy.Legacy)

		second, _ := keysetPage(t, db, true, legacy, 10)
		require.NotEmpty(t, second)
		assert.Equal(t, all[0], second[0].ID, "the next page starts again at the second's newest row")

		seen := map[string]bool{}
		for _, r := range append(first, second...) {
			seen[r.ID] = true
		}

		for _, id := range all {
			assert.True(t, seen[id], "row %s is never skipped", id)
		}
	})

	t.Run("ascending", func(t *testing.T) {
		legacy, err := pagination.DecodeKeyset(pagination.EncodeCursor(rows[0].CreatedAt))
		require.NoError(t, err)

		page, _ := keysetPage(t, db, false, legacy, 10)
		require.Len(t, page, len(rows))
		assert.Equal(t, keysetOrder(rows, false)[0], page[0].ID, "the second's oldest row comes first")
	})
}

// keysetSQL renders the SQL of a Find on table with WithKeyset applied.
func keysetSQL(t *testing.T, table, column string, desc bool, k pagination.Keyset) string {
	t.Helper()

	db := newKeysetDB(t)

	return db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var rows []keysetRow

		return models.ApplyOptions(tx.Table(table), models.WithKeyset(column, desc, k)).Find(&rows)
	})
}

func TestWithKeyset_BuildsItsBoundAndOrder(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 3, 1, 12, 0, 0, 250_000_000, time.UTC)
	position := pagination.Keyset{At: at, ID: "row-a"}
	legacy := pagination.Keyset{At: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), Legacy: true}

	t.Run("descending", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows", "created_at", true, position)
		assert.Contains(t, sql, "(`created_at`, `id`) < (")
		assert.Contains(t, sql, "ORDER BY `created_at` DESC,`id` DESC")
	})

	t.Run("ascending", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows", "created_at", false, position)
		assert.Contains(t, sql, "(`created_at`, `id`) > (")
		assert.Contains(t, sql, "ORDER BY `created_at` ASC,`id` ASC")
	})

	t.Run("the first page has no bound", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows", "created_at", true, pagination.Keyset{})
		assert.NotContains(t, sql, "WHERE")
		assert.Contains(t, sql, "ORDER BY `created_at` DESC,`id` DESC")
	})

	t.Run("a legacy position, descending, runs to the end of its second", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows", "created_at", true, legacy)
		assert.Contains(t, sql, "`created_at` < \"2026-03-01 12:00:01\"")
		assert.NotContains(t, sql, "`id`) <")
	})

	t.Run("a legacy position, ascending, starts at its second", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows", "created_at", false, legacy)
		assert.Contains(t, sql, "`created_at` >= \"2026-03-01 12:00:00\"")
	})

	t.Run("a table-qualified column", func(t *testing.T) {
		t.Parallel()

		sql := keysetSQL(t, "keyset_rows AS t", "t.created_at", true, position)
		assert.Contains(t, sql, "(`t`.`created_at`, `t`.`id`) < (")
		assert.Contains(t, sql, "ORDER BY `t`.`created_at` DESC,`t`.`id` DESC")
	})
}

func TestWithKeyset_RefusesAColumnThatIsNotAName(t *testing.T) {
	t.Parallel()

	columns := []string{
		"created_at DESC",
		"created_at; drop table x",
		"a.b.c",
		"t.",
		".c",
		"1st",
		"created-at",
		"(select 1)",
		"",
	}

	for _, column := range columns {
		t.Run(column, func(t *testing.T) {
			t.Parallel()

			rec := &statementRecorder{}
			db := newKeysetDB(t).Session(&gorm.Session{Logger: rec})

			var rows []keysetRow
			err := models.ApplyOptions(
				db.Model(&keysetRow{}),
				models.WithKeyset(column, true, pagination.Keyset{}),
			).Limit(10).Find(&rows).Error

			require.ErrorIs(t, err, models.ErrValidation)

			var verr *models.ValidationError
			assert.NotErrorAs(t, err, &verr, "a column is the developer's, not a field to report")

			if column != "" {
				assert.NotContains(t, err.Error(), column, "the error must never echo the column")
			}

			assert.Empty(t, rec.statements(), "a refused query sends nothing")
		})
	}
}

func TestWithKeyset_RefusalStaysWithItsQuery(t *testing.T) {
	t.Parallel()

	db := newKeysetDB(t)
	seedKeysetRows(t, db, sameSecondRows())

	shared := db.WithContext(t.Context())

	var rows []keysetRow
	err := models.ApplyOptions(shared, models.WithKeyset("created at", true, pagination.Keyset{})).Find(&rows).Error
	require.ErrorIs(t, err, models.ErrValidation)

	rows = nil
	require.NoError(t, shared.Find(&rows).Error, "the refusal belongs to its own query, not the shared handle")
	assert.Len(t, rows, len(sameSecondRows()))
}
