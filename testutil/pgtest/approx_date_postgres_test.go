//go:build integration

package pgtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
	"github.com/mrz1836/go-foundation/testutil/pgtest"
)

// approxRecord is a model with a required and an optional approximate date.
type approxRecord struct {
	ID    uint
	EDTF  models.ApproxDate `gorm:"column:edtf;not null"`
	Maybe *models.ApproxDate
}

// TestApproxDate_PostgresRoundTrip writes every EDTF form to text columns under
// a session time zone fourteen hours ahead of UTC, and reads them back under it
// and under the connection's default: each must read back as the same value
// and the same text.
func TestApproxDate_PostgresRoundTrip(t *testing.T) {
	db := pgtest.NewPostgresIsolatedDB(t)
	require.NoError(t, db.AutoMigrate(&approxRecord{}))

	var columns []struct {
		ColumnName string
		DataType   string
	}
	require.NoError(t, db.Raw(
		"SELECT column_name, data_type FROM information_schema.columns "+
			"WHERE table_schema = current_schema() AND table_name = 'approx_records' "+
			"AND column_name IN ('edtf', 'maybe') ORDER BY column_name",
	).Scan(&columns).Error)
	require.Len(t, columns, 2)

	for _, c := range columns {
		assert.Equal(t, "text", c.DataType, c.ColumnName)
	}

	texts := []string{
		"1998-03-14", "1998-03", "1998", "199X",
		"1998~", "1998-03~", "1998-03-14~", "199X~",
		"[..1998]", "[..1998-03-14]", "[..199X]", "[1998..]",
		"[1995..1998]", "[1998-03..1999-02]", "[1984-11-26..1985-11-25]", "[198X..199X]",
		"XXXX",
	}

	written := make([]approxRecord, 0, len(texts))

	for _, s := range texts {
		a, err := models.ParseEDTF(s)
		require.NoError(t, err, s)

		written = append(written, approxRecord{EDTF: a})
	}

	maybe, err := models.ParseEDTF("[1999-03-01..2000-02-29]")
	require.NoError(t, err)

	written[0].Maybe = &maybe

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		setSessionTimeZone(t, tx, "Pacific/Kiritimati")

		for i := range written {
			if createErr := tx.Create(&written[i]).Error; createErr != nil {
				return createErr
			}
		}

		return nil
	}))

	readBack := func(tx *gorm.DB) {
		var read []approxRecord
		require.NoError(t, tx.Order("id").Find(&read).Error)
		assert.Equal(t, written, read, "every form reads back equal, and a nil pointer reads back nil")

		var stored []string
		require.NoError(t, tx.Raw("SELECT edtf FROM approx_records ORDER BY id").Scan(&stored).Error)
		assert.Equal(t, texts, stored, "the column holds the EDTF text")
	}

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		setSessionTimeZone(t, tx, "Pacific/Kiritimati")
		readBack(tx)

		return nil
	}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		readBack(tx)

		return nil
	}))

	// The error is matched by sentinel, never by text: the driver builds its
	// message from the value's fields.
	err = db.Create(&approxRecord{}).Error
	require.ErrorIs(t, err, models.ErrInvalidApproxDate)

	var count int64
	require.NoError(t, db.Model(&approxRecord{}).Count(&count).Error)
	assert.Equal(t, int64(len(written)), count, "a refused value writes no row")
}
