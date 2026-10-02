//go:build integration

package pgtest_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// datedRecord is a model with a required and an optional civil date.
type datedRecord struct {
	ID    uint
	Born  models.Date `gorm:"not null"`
	Maybe *models.Date
}

// setSessionTimeZone sets the time zone for the rest of tx and checks that it
// took.
func setSessionTimeZone(t *testing.T, tx *gorm.DB, zone string) {
	t.Helper()

	require.NoError(t, tx.Exec("SET LOCAL TIME ZONE '"+zone+"'").Error)

	var shown string
	require.NoError(t, tx.Raw("SHOW TIME ZONE").Scan(&shown).Error)
	require.Equal(t, zone, shown)
}

// TestDate_PostgresRoundTripInAnySessionTimeZone writes dates under a session
// time zone fourteen hours ahead of UTC and reads them under one eleven hours
// behind, and under the connection's default: a date column holds no zone, so
// each must read back as the same day.
func TestDate_PostgresRoundTripInAnySessionTimeZone(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&datedRecord{}))

	var columns []struct {
		ColumnName string
		DataType   string
	}
	require.NoError(t, db.Raw(
		"SELECT column_name, data_type FROM information_schema.columns "+
			"WHERE table_schema = current_schema() AND table_name = 'dated_records' "+
			"AND column_name IN ('born', 'maybe') ORDER BY column_name",
	).Scan(&columns).Error)
	require.Len(t, columns, 2)

	for _, c := range columns {
		assert.Equal(t, "date", c.DataType, c.ColumnName)
	}

	parse := func(s string) models.Date {
		d, err := models.ParseDate(s)
		require.NoError(t, err)

		return d
	}

	maybe := parse("2000-02-29")
	written := []datedRecord{
		{Born: parse("1900-01-01")},
		{Born: parse("2000-02-29"), Maybe: &maybe},
		{Born: parse("0001-01-01")},
		{Born: parse("9999-12-31")},
	}

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		setSessionTimeZone(t, tx, "Pacific/Kiritimati")

		for i := range written {
			if err := tx.Create(&written[i]).Error; err != nil {
				return err
			}
		}

		return nil
	}))

	readBack := func(tx *gorm.DB) {
		var read []datedRecord
		require.NoError(t, tx.Order("id").Find(&read).Error)
		assert.Equal(t, written, read)

		var texts []string
		require.NoError(t, tx.Raw("SELECT born::text FROM dated_records ORDER BY id").Scan(&texts).Error)
		assert.Equal(t, []string{"1900-01-01", "2000-02-29", "0001-01-01", "9999-12-31"}, texts)
	}

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		setSessionTimeZone(t, tx, "Pacific/Pago_Pago")
		readBack(tx)

		return nil
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		readBack(tx)

		return nil
	}))

	// The error is matched by sentinel, never by text: pgx builds its message
	// from the value's fields.
	for _, born := range []models.Date{{}, {Year: 2026, Month: time.February, Day: 30}} {
		err := db.Create(&datedRecord{Born: born}).Error
		require.ErrorIs(t, err, models.ErrInvalidDate)
	}

	var count int64
	require.NoError(t, db.Model(&datedRecord{}).Count(&count).Error)
	assert.Equal(t, int64(len(written)), count, "a refused date writes no row")
}
