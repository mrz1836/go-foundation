package models_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// sortRow is a list row a client may sort by its creation time or its name.
type sortRow struct {
	ID          string    `gorm:"primaryKey"`
	CreatedAt   time.Time `gorm:"not null"`
	DisplayName string    `gorm:"not null"`
}

// sortKeyName is the client-facing sort key for a row's display name.
const sortKeyName = "name"

// sortAllowlist maps the sort keys a client may send to their columns.
func sortAllowlist() map[string]string {
	return map[string]string{"created_at": "created_at", sortKeyName: "display_name"}
}

// newSortDB opens an in-memory SQLite database on one connection, with sortRow
// migrated.
func newSortDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open sqlite test db")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&sortRow{}), "migrate sort rows")

	return db
}

func TestParseSort_ReadsKeysAgainstTheAllowlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		want []models.SortField
	}{
		{
			name: "two keys, the first descending",
			expr: "-created_at,name",
			want: []models.SortField{
				{Key: "created_at", Column: "created_at", Desc: true},
				{Key: sortKeyName, Column: "display_name", Desc: false},
			},
		},
		{
			name: "one key, ascending",
			expr: sortKeyName,
			want: []models.SortField{{Key: sortKeyName, Column: "display_name"}},
		},
		{name: "no sort", expr: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := models.ParseSort(tt.expr, sortAllowlist())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSort_RefusesUnknownRepeatedEmptyOrSpacedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr string
		want string
	}{
		{expr: "-created_at,created_at", want: "a sort key appears more than once"},
		{expr: "name,name", want: "a sort key appears more than once"},
		{expr: "color", want: "unknown sort key; allowed: created_at, name"},
		{expr: "--name", want: "unknown sort key; allowed: created_at, name"},
		{expr: "name,,created_at", want: "empty sort key"},
		{expr: "name,", want: "empty sort key"},
		{expr: "-", want: "empty sort key"},
		{expr: "name ", want: "sort keys must not contain spaces"},
		{expr: " name", want: "sort keys must not contain spaces"},
		{expr: "name,\tcreated_at", want: "sort keys must not contain spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()

			got, err := models.ParseSort(tt.expr, sortAllowlist())
			assert.Nil(t, got)

			var verr *models.ValidationError
			require.ErrorAs(t, err, &verr)
			assert.Equal(t, "sort", verr.Field)
			assert.Equal(t, tt.want, verr.Message)
			assert.NotContains(t, err.Error(), tt.expr, "the error must never echo the input")
		})
	}
}

func TestWithSort_OrdersByEachColumnInTurn(t *testing.T) {
	t.Parallel()

	db := newSortDB(t)
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create([]sortRow{
		{ID: "row-1", CreatedAt: at, DisplayName: "beta"},
		{ID: "row-2", CreatedAt: at.Add(time.Second), DisplayName: "gamma"},
		{ID: "row-3", CreatedAt: at, DisplayName: "alpha"},
		{ID: "row-4", CreatedAt: at.Add(time.Second), DisplayName: "delta"},
	}).Error)

	fields, err := models.ParseSort("-created_at,name", sortAllowlist())
	require.NoError(t, err)

	repo := models.NewRepository[sortRow, string](db)
	rows, err := repo.FindAll(t.Context(), models.WithSort(fields))
	require.NoError(t, err)

	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}

	assert.Equal(t, []string{"row-4", "row-2", "row-3", "row-1"}, ids)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var dest []sortRow

		return models.ApplyOptions(tx.Model(&sortRow{}), models.WithSort(fields)).Find(&dest)
	})
	assert.Contains(t, sql, "ORDER BY `created_at` DESC,`display_name` ASC")
	assert.NotContains(t, sql, "`id`", "WithSort adds no tie-breaker")
}

func TestWithSort_NoFieldsOrdersNothing(t *testing.T) {
	t.Parallel()

	db := newSortDB(t)

	for _, fields := range [][]models.SortField{nil, {}} {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			var dest []sortRow

			return models.ApplyOptions(tx.Model(&sortRow{}), models.WithSort(fields)).Find(&dest)
		})
		assert.NotContains(t, sql, "ORDER BY")
	}
}

func TestWithSort_RefusesAColumnThatIsNotAName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields []models.SortField
	}{
		{name: "a statement", fields: []models.SortField{{Key: "created_at", Column: "created_at; DROP TABLE sort_rows"}}},
		{name: "a bad column after a good one", fields: []models.SortField{
			{Key: sortKeyName, Column: "display_name"},
			{Key: "created_at", Column: "created_at DESC"},
		}},
		{name: "an empty column", fields: []models.SortField{{Key: sortKeyName}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newSortDB(t)
			rec := &statementRecorder{}
			repo := models.NewRepository[sortRow, string](db.Session(&gorm.Session{Logger: rec}))

			_, err := repo.FindAll(t.Context(), models.WithSort(tt.fields))
			require.ErrorIs(t, err, models.ErrValidation)
			assert.Empty(t, rec.statements(), "a refused query sends nothing")
			assert.True(t, db.Migrator().HasTable(&sortRow{}), "the table is intact")
		})
	}
}

func BenchmarkParseSort(b *testing.B) {
	allowed := map[string]string{
		"created_at": "created_at",
		sortKeyName:  "display_name",
		"status":     "status",
		"updated_at": "updated_at",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, _ = models.ParseSort("-created_at,name", allowed)
	}
}
