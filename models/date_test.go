package models_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mrz1836/go-foundation/models"
)

// leapDay is the date most cases use: it exists only in leap years, so a
// conversion that slips a day shows.
const leapDay = "2000-02-29"

// datedRow is a model with a required and an optional civil date.
type datedRow struct {
	ID    uint
	Born  models.Date `gorm:"not null"`
	Maybe *models.Date
}

// newDateDB opens an in-memory SQLite database holding dated_rows.
func newDateDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Every pooled connection to ":memory:" is its own empty database, so the
	// pool keeps exactly one.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&datedRow{}))

	return db
}

// mustParseDate parses s, failing the test when it is not a date.
func mustParseDate(t *testing.T, s string) models.Date {
	t.Helper()

	d, err := models.ParseDate(s)
	require.NoError(t, err)

	return d
}

func TestParseDate_AcceptsCalendarDates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in    string
		year  int
		month time.Month
		day   int
	}{
		{in: "2024-02-29", year: 2024, month: time.February, day: 29},
		{in: leapDay, year: 2000, month: time.February, day: 29},
		{in: "1900-01-01", year: 1900, month: time.January, day: 1},
		{in: "0001-01-01", year: 1, month: time.January, day: 1},
		{in: "9999-12-31", year: 9999, month: time.December, day: 31},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			d, err := models.ParseDate(tc.in)
			require.NoError(t, err)
			assert.Equal(t, models.Date{Year: tc.year, Month: tc.month, Day: tc.day}, d)
			assert.Equal(t, tc.in, d.String())
		})
	}

	built, err := models.NewDate(2024, time.February, 29)
	require.NoError(t, err)
	assert.Equal(t, mustParseDate(t, "2024-02-29"), built)
}

func TestParseDate_RefusesAnythingElse(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"2026-02-30",
		"2023-02-29",
		"1900-02-29",
		"2026-13-01",
		"2026-00-10",
		"2026-01-00",
		"2026-01-32",
		"0000-01-01",
		"10000-01-01",
		"2026-1-1",
		"26-01-01",
		"20260101",
		"2026/01/01",
		"2026-01-01T00:00:00Z",
		"2026-01-01 00:00:00",
		" 2026-01-01",
		"2026-01-01 ",
		"+2026-01-01",
		"\uff12\uff10\uff12\uff16-01-01", // fullwidth digits
		"yesterday",
		"",
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			d, err := models.ParseDate(in)
			assert.Equal(t, models.Date{}, d)
			require.ErrorIs(t, err, models.ErrInvalidDate)
			require.EqualError(t, err, models.ErrInvalidDate.Error(), "the error text is fixed and never holds the input")
		})
	}
}

func TestNewDate_RefusesImpossibleDates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		year  int
		month time.Month
		day   int
	}{
		{year: 2023, month: time.February, day: 29},
		{year: 2026, month: time.April, day: 31},
		{year: 2026, month: 0, day: 1},
		{year: 2026, month: 13, day: 1},
		{year: 2026, month: time.January, day: 0},
		{year: 0, month: time.January, day: 1},
		{year: 10000, month: time.January, day: 1},
	}

	for _, tc := range cases {
		d, err := models.NewDate(tc.year, tc.month, tc.day)
		assert.Equal(t, models.Date{}, d)
		require.ErrorIs(t, err, models.ErrInvalidDate, "%d-%d-%d", tc.year, tc.month, tc.day)
	}
}

func TestDateOf_IsTheUTCCalendarDay(t *testing.T) {
	t.Parallel()

	minusFive := time.FixedZone("UTC-5", -5*60*60)
	plusFourteen := time.FixedZone("UTC+14", 14*60*60)

	assert.Equal(t, "2026-03-02", models.DateOf(time.Date(2026, 3, 1, 23, 30, 0, 0, minusFive)).String())
	assert.Equal(t, "2026-03-01", models.DateOf(time.Date(2026, 3, 2, 0, 30, 0, 0, plusFourteen)).String())
}

func TestDate_CompareAndZero(t *testing.T) {
	t.Parallel()

	earlier := mustParseDate(t, "1999-12-31")
	later := mustParseDate(t, "2000-01-01")

	assert.True(t, earlier.Before(later))
	assert.False(t, earlier.After(later))
	assert.True(t, later.After(earlier))
	assert.False(t, later.Before(earlier))
	assert.False(t, earlier.Before(earlier), "a date is not before itself")
	assert.False(t, earlier.After(earlier), "a date is not after itself")

	assert.True(t, models.Date{}.IsZero())
	assert.False(t, mustParseDate(t, leapDay).IsZero())
}

func TestDate_ValueWritesISOText(t *testing.T) {
	t.Parallel()

	v, err := mustParseDate(t, leapDay).Value()
	require.NoError(t, err)
	assert.Equal(t, leapDay, v, "the driver value is YYYY-MM-DD text")

	for _, d := range []models.Date{{}, {Year: 2026, Month: time.February, Day: 30}} {
		v, err = d.Value()
		require.ErrorIs(t, err, models.ErrInvalidDate)
		assert.Nil(t, v)
	}
}

func TestDate_ScanReadsTimesStringsAndBytes(t *testing.T) {
	t.Parallel()

	plusFourteen := time.FixedZone("UTC+14", 14*60*60)
	want := mustParseDate(t, leapDay)

	for name, src := range map[string]any{
		"time keeps its own day": time.Date(2000, 2, 29, 0, 0, 0, 0, plusFourteen),
		"string":                 leapDay,
		"bytes":                  []byte(leapDay),
		"timestamp bytes":        []byte("2000-02-29 00:00:00+00:00"),
		"timestamp string":       "2000-02-29T00:00:00Z",
	} {
		var d models.Date
		require.NoError(t, d.Scan(src), name)
		assert.Equal(t, want, d, name)
	}

	preset := mustParseDate(t, "1999-12-31")

	for name, src := range map[string]any{
		"nil":            nil,
		"number":         int64(20000229),
		"not a date":     "not a date",
		"impossible day": "2026-02-30",
		"year zero time": time.Date(0, time.January, 1, 0, 0, 0, 0, time.UTC),
	} {
		d := preset
		require.ErrorIs(t, d.Scan(src), models.ErrInvalidDate, name)
		assert.Equal(t, preset, d, "%s: a failed scan leaves the date unchanged", name)
	}
}

func TestDate_TextAndJSON(t *testing.T) {
	t.Parallel()

	type record struct {
		Born  models.Date  `json:"born"`
		Maybe *models.Date `json:"maybe"`
		Skip  models.Date  `json:"skip,omitzero"`
	}

	encoded, err := json.Marshal(record{Born: mustParseDate(t, leapDay)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"born":"2000-02-29","maybe":null}`, string(encoded))
	assert.NotContains(t, string(encoded), "skip", "omitzero omits the zero Date")

	_, err = json.Marshal(record{})
	require.ErrorIs(t, err, models.ErrInvalidDate, "the zero Date is not a date")

	for _, d := range []models.Date{{}, {Year: 2026, Month: time.February, Day: 30}} {
		_, err = d.MarshalText()
		require.ErrorIs(t, err, models.ErrInvalidDate)
	}

	var decoded record
	err = json.Unmarshal([]byte(`{"born":"2026-02-30"}`), &decoded)
	require.ErrorIs(t, err, models.ErrInvalidDate)
	assert.NotContains(t, err.Error(), "2026-02-30", "the error never repeats the value")

	text, err := mustParseDate(t, leapDay).MarshalText()
	require.NoError(t, err)
	assert.Equal(t, leapDay, string(text))

	var back models.Date
	require.NoError(t, back.UnmarshalText(text))
	assert.Equal(t, mustParseDate(t, leapDay), back)
}

func TestDate_SQLiteRoundTrip(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "date", models.Date{}.GormDataType())

	db := newDateDB(t)

	var ddl string
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'dated_rows'").Scan(&ddl).Error)
	assert.Contains(t, ddl, "`born` date NOT NULL")
	assert.Contains(t, ddl, "`maybe` date")

	maybe := mustParseDate(t, leapDay)
	written := []datedRow{
		{Born: mustParseDate(t, "1900-01-01")},
		{Born: mustParseDate(t, leapDay), Maybe: &maybe},
		{Born: mustParseDate(t, "0001-01-01")},
		{Born: mustParseDate(t, "9999-12-31")},
	}

	for i := range written {
		require.NoError(t, db.Create(&written[i]).Error)
	}

	var read []datedRow
	require.NoError(t, db.Order("id").Find(&read).Error)
	assert.Equal(t, written, read)

	for _, born := range []models.Date{{}, {Year: 2026, Month: time.February, Day: 30}} {
		err := db.Create(&datedRow{Born: born}).Error
		require.ErrorIs(t, err, models.ErrInvalidDate)
	}

	var count int64
	require.NoError(t, db.Model(&datedRow{}).Count(&count).Error)
	assert.Equal(t, int64(len(written)), count, "a refused date writes no row")
}

// FuzzParseDate proves ParseDate never panics, refuses with exactly
// ErrInvalidDate and the zero Date, and accepts only text that it, String, and
// Value give back unchanged.
func FuzzParseDate(f *testing.F) {
	for _, seed := range []string{
		"2024-02-29",
		"2026-02-30",
		"0000-01-01",
		"9999-12-31",
		" 2026-01-01",
		"2026-01-01T00:00:00Z",
		"",
		"\xff",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		d, err := models.ParseDate(in)
		if err != nil {
			if !errors.Is(err, models.ErrInvalidDate) || err.Error() != models.ErrInvalidDate.Error() || !d.IsZero() {
				t.Fatalf("ParseDate refused with %v and %+v", err, d)
			}

			return
		}

		checkParsedDate(t, in, d)
	})
}

// checkParsedDate fails t unless d writes back exactly as in and rebuilds to
// itself.
func checkParsedDate(t *testing.T, in string, d models.Date) {
	t.Helper()

	if d.String() != in {
		t.Fatalf("ParseDate(%q).String() = %q", in, d.String())
	}

	if v, err := d.Value(); err != nil || v != in {
		t.Fatalf("ParseDate(%q).Value() = %v, %v", in, v, err)
	}

	if rebuilt, err := models.NewDate(d.Year, d.Month, d.Day); err != nil || rebuilt != d {
		t.Fatalf("NewDate(%d, %d, %d) = %+v, %v", d.Year, d.Month, d.Day, rebuilt, err)
	}
}
