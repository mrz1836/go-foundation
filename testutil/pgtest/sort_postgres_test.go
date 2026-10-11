//go:build integration

package pgtest_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// sortPgRow is a list row a client may sort by its creation time or its name.
type sortPgRow struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
	DisplayName string    `gorm:"not null"`
}

// pgStatementRecorder is a GORM logger that keeps the SQL of every statement
// GORM traces, so a test can prove that a refused query sent nothing.
type pgStatementRecorder struct {
	mu   sync.Mutex
	sqls []string
}

// LogMode returns the recorder itself; it records at every level.
func (r *pgStatementRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

// Info discards the message.
func (r *pgStatementRecorder) Info(context.Context, string, ...any) {}

// Warn discards the message.
func (r *pgStatementRecorder) Warn(context.Context, string, ...any) {}

// Error discards the message.
func (r *pgStatementRecorder) Error(context.Context, string, ...any) {}

// Trace records the statement's SQL.
func (r *pgStatementRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sqls = append(r.sqls, sql)
}

// count returns how many statements were recorded.
func (r *pgStatementRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.sqls)
}

func TestSort_PostgresOrdersByQuotedColumns(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&sortPgRow{}))

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	rows := []sortPgRow{
		{ID: models.NewID(), CreatedAt: at, DisplayName: "beta"},
		{ID: models.NewID(), CreatedAt: at.Add(time.Second), DisplayName: "gamma"},
		{ID: models.NewID(), CreatedAt: at, DisplayName: "alpha"},
		{ID: models.NewID(), CreatedAt: at.Add(time.Second), DisplayName: "delta"},
	}
	require.NoError(t, db.Create(&rows).Error)

	names := func(found []sortPgRow) []string {
		out := make([]string, 0, len(found))
		for _, r := range found {
			out = append(out, r.DisplayName)
		}

		return out
	}

	t.Run("WithSort through a table alias", func(t *testing.T) {
		fields, err := models.ParseSort("-created_at,name", map[string]string{
			"created_at": "t.created_at",
			"name":       "t.display_name",
		})
		require.NoError(t, err)

		var found []sortPgRow
		require.NoError(t, models.ApplyOptions(db.Table("sort_pg_rows AS t"), models.WithSort(fields)).Find(&found).Error)
		assert.Equal(t, []string{"delta", "gamma", "alpha", "beta"}, names(found))
	})

	t.Run("WithOrderBy through a table alias", func(t *testing.T) {
		var found []sortPgRow
		require.NoError(t, models.ApplyOptions(db.Table("sort_pg_rows AS t"),
			models.WithOrderBy("t.created_at", true),
			models.WithOrderBy("t.display_name", false),
		).Find(&found).Error)
		assert.Equal(t, []string{"delta", "gamma", "alpha", "beta"}, names(found))
	})

	t.Run("a refused column sends nothing", func(t *testing.T) {
		rec := &pgStatementRecorder{}
		quiet := db.Session(&gorm.Session{Logger: rec})

		var found []sortPgRow
		err := models.ApplyOptions(quiet.Model(&sortPgRow{}),
			models.WithSort([]models.SortField{{Key: "name", Column: "display_name; DROP TABLE sort_pg_rows"}}),
		).Find(&found).Error
		require.ErrorIs(t, err, models.ErrValidation)
		assert.Zero(t, rec.count(), "a refused query sends nothing")

		var count int64
		require.NoError(t, db.Model(&sortPgRow{}).Count(&count).Error, "the table is intact")
		assert.Equal(t, int64(len(rows)), count)
	})
}
