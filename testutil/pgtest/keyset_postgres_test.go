//go:build integration

package pgtest_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/pagination"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// keysetPgRow is a list row with a uuid id and a timestamptz creation time.
type keysetPgRow struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
	Label     string
}

// newKeysetPgDB returns a private schema holding keyset_pg_rows, seeded with
// rows at the given times, each with a fresh UUID v7 id.
func newKeysetPgDB(t *testing.T, times ...time.Time) (*gorm.DB, []keysetPgRow) {
	t.Helper()

	db := pgtest.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&keysetPgRow{}))

	rows := make([]keysetPgRow, 0, len(times))
	for _, at := range times {
		rows = append(rows, keysetPgRow{ID: models.NewID(), CreatedAt: at})
	}

	if len(rows) > 0 {
		require.NoError(t, db.Create(&rows).Error)
	}

	return db, rows
}

// keysetPgOrder returns the rows' ids in (created_at, id) order.
func keysetPgOrder(rows []keysetPgRow, desc bool) []string {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b keysetPgRow) int {
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

// keysetPgPage reads one page after k from query: size rows, plus whether
// another follows.
func keysetPgPage(t *testing.T, query func() *gorm.DB, column string, desc bool, k pagination.Keyset, size int) ([]keysetPgRow, bool) {
	t.Helper()

	var rows []keysetPgRow
	require.NoError(t, models.ApplyOptions(query(), models.WithKeyset(column, desc, k)).Limit(size+1).Find(&rows).Error)

	if len(rows) > size {
		return rows[:size], true
	}

	return rows, false
}

// keysetPgPageAll reads every page as a client would, through cursor strings.
func keysetPgPageAll(t *testing.T, query func() *gorm.DB, column string, desc bool, size int) []string {
	t.Helper()

	var (
		ids []string
		k   pagination.Keyset
	)

	for range 100 { // a bound, so a paging bug fails instead of looping
		rows, more := keysetPgPage(t, query, column, desc, k, size)
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

// keysetPgSecond returns an instant inside 12:00 on a fixed day, in UTC.
func keysetPgSecond(sec, nanos int) time.Time {
	return time.Date(2026, 3, 1, 12, 0, sec, nanos, time.UTC)
}

func TestWithKeyset_PostgresPagesWithoutSkipping(t *testing.T) {
	t.Run("rows sharing a second", func(t *testing.T) {
		db, rows := newKeysetPgDB(t,
			keysetPgSecond(0, 100_000_000), keysetPgSecond(0, 200_000_000),
			keysetPgSecond(0, 300_000_000), keysetPgSecond(0, 400_000_000),
			keysetPgSecond(-1, 900_000_000), keysetPgSecond(1, 0),
		)
		query := func() *gorm.DB { return db.Model(&keysetPgRow{}) }

		for _, desc := range []bool{true, false} {
			for _, size := range []int{1, 2, 3} {
				assert.Equal(t, keysetPgOrder(rows, desc), keysetPgPageAll(t, query, "created_at", desc, size),
					"desc=%v size=%d", desc, size)
			}
		}
	})

	t.Run("one timestamp by id", func(t *testing.T) {
		at := keysetPgSecond(0, 250_000_000)
		db, rows := newKeysetPgDB(t, at, at, at, at, at, at, at)
		query := func() *gorm.DB { return db.Model(&keysetPgRow{}) }

		for _, desc := range []bool{true, false} {
			assert.Equal(t, keysetPgOrder(rows, desc), keysetPgPageAll(t, query, "created_at", desc, 2), "desc=%v", desc)
		}
	})

	t.Run("microseconds", func(t *testing.T) {
		older, newer := keysetPgSecond(0, 500_000_000), keysetPgSecond(0, 500_001_000)
		db, rows := newKeysetPgDB(t, older, newer)
		query := func() *gorm.DB { return db.Model(&keysetPgRow{}) }

		first, more := keysetPgPage(t, query, "created_at", true, pagination.Keyset{}, 1)
		require.True(t, more)
		require.Len(t, first, 1)

		k, err := pagination.DecodeKeyset(pagination.EncodeKeyset(first[0].CreatedAt, first[0].ID))
		require.NoError(t, err)

		second, _ := keysetPgPage(t, query, "created_at", true, k, 1)
		require.Len(t, second, 1)
		assert.Equal(t, keysetPgOrder(rows, true)[1], second[0].ID, "the next page starts with the older row")
	})

	t.Run("a table-qualified column", func(t *testing.T) {
		db, rows := newKeysetPgDB(t,
			keysetPgSecond(0, 100_000_000), keysetPgSecond(0, 200_000_000),
			keysetPgSecond(0, 200_000_000), keysetPgSecond(1, 0),
		)
		query := func() *gorm.DB { return db.Table("keyset_pg_rows AS r") }

		assert.Equal(t, keysetPgOrder(rows, true), keysetPgPageAll(t, query, "r.created_at", true, 1))
	})
}

func TestWithKeyset_PostgresLegacyCursorRepeatsButNeverSkips(t *testing.T) {
	db, rows := newKeysetPgDB(t,
		keysetPgSecond(0, 100_000_000), keysetPgSecond(0, 200_000_000),
		keysetPgSecond(0, 300_000_000), keysetPgSecond(0, 400_000_000),
	)
	query := func() *gorm.DB { return db.Model(&keysetPgRow{}) }
	all := keysetPgOrder(rows, true)

	first, more := keysetPgPage(t, query, "created_at", true, pagination.Keyset{}, 2)
	require.True(t, more)

	legacy, err := pagination.DecodeKeyset(pagination.EncodeCursor(first[len(first)-1].CreatedAt))
	require.NoError(t, err)
	require.True(t, legacy.Legacy)

	second, _ := keysetPgPage(t, query, "created_at", true, legacy, 10)
	require.NotEmpty(t, second)
	assert.Equal(t, all[0], second[0].ID, "the next page starts again at the second's newest row")

	seen := map[string]bool{}
	for _, r := range append(first, second...) {
		seen[r.ID] = true
	}

	for _, id := range all {
		assert.True(t, seen[id], "row %s is never skipped", id)
	}

	ascending, _ := keysetPgPage(t, query, "created_at", false, legacy, 10)
	require.Len(t, ascending, len(rows))
	assert.Equal(t, keysetPgOrder(rows, false)[0], ascending[0].ID, "ascending, the second's oldest row comes first")
}
