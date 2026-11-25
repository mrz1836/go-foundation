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

// The days most cases name.
const (
	dayInMarch = "1998-03-14" // a day inside its month, year, and decade
	start1990s = "1990-01-01" // the first day of the 1990s
	end1990s   = "1999-12-31" // the last day of the 1990s
)

// boundOf parses s as a bound: the empty string is an open bound, the zero
// Date.
func boundOf(t *testing.T, s string) models.Date {
	t.Helper()

	if s == "" {
		return models.Date{}
	}

	return mustParseDate(t, s)
}

func TestDatePrecision_Valid(t *testing.T) {
	t.Parallel()

	for want, p := range map[string]models.DatePrecision{
		"day":     models.DatePrecisionDay,
		"month":   models.DatePrecisionMonth,
		"year":    models.DatePrecisionYear,
		"decade":  models.DatePrecisionDecade,
		"unknown": models.DatePrecisionUnknown,
	} {
		assert.Equal(t, want, string(p), "each precision's text is stable")
		assert.True(t, p.Valid(), want)
	}

	for _, p := range []models.DatePrecision{"", "week", "Day", "day "} {
		assert.False(t, p.Valid(), "%q is not a precision", string(p))
	}
}

func TestDatePrecision_PeriodOf(t *testing.T) {
	t.Parallel()

	day, month := models.DatePrecisionDay, models.DatePrecisionMonth
	year, decade := models.DatePrecisionYear, models.DatePrecisionDecade

	cases := []struct {
		name      string
		precision models.DatePrecision
		in        string
		first     string
		last      string
	}{
		{"a day is itself", day, dayInMarch, dayInMarch, dayInMarch},
		{"a month, from any of its days", month, dayInMarch, "1998-03-01", "1998-03-31"},
		{"a leap February", month, "2000-02-10", "2000-02-01", leapDay},
		{"a year", year, "1998-07-04", "1998-01-01", "1998-12-31"},
		{"a decade starts in a year ending in 0", decade, "1998-07-04", start1990s, end1990s},
		{"a decade from its first day", decade, start1990s, start1990s, end1990s},
		{"the first whole decade", decade, "0015-05-05", "0010-01-01", "0019-12-31"},
		{"the last decade", decade, "9995-01-01", "9990-01-01", "9999-12-31"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			first, last, ok := tc.precision.PeriodOf(mustParseDate(t, tc.in))
			assert.True(t, ok)
			assert.Equal(t, mustParseDate(t, tc.first), first)
			assert.Equal(t, mustParseDate(t, tc.last), last)
		})
	}

	refused := []struct {
		name      string
		precision models.DatePrecision
		in        models.Date
	}{
		{"unknown names no period", models.DatePrecisionUnknown, mustParseDate(t, dayInMarch)},
		{"a precision that is not one", "week", mustParseDate(t, dayInMarch)},
		{"the zero date", day, models.Date{}},
		{"a day that does not exist", month, models.Date{Year: 2026, Month: 2, Day: 30}},
		{"the decade of the years 1 to 9 starts in the year 0", decade, mustParseDate(t, "0005-06-07")},
	}

	for _, tc := range refused {
		first, last, ok := tc.precision.PeriodOf(tc.in)
		assert.False(t, ok, tc.name)
		assert.Equal(t, models.Date{}, first, tc.name)
		assert.Equal(t, models.Date{}, last, tc.name)
	}
}

func TestDateQualifier_Valid(t *testing.T) {
	t.Parallel()

	for want, q := range map[string]models.DateQualifier{
		"exact":   models.DateQualifierExact,
		"about":   models.DateQualifierAbout,
		"before":  models.DateQualifierBefore,
		"after":   models.DateQualifierAfter,
		"between": models.DateQualifierBetween,
	} {
		assert.Equal(t, want, string(q), "each qualifier's text is stable")
		assert.True(t, q.Valid(), want)
	}

	for _, q := range []models.DateQualifier{"", "circa", "Exact", "exact "} {
		assert.False(t, q.Valid(), "%q is not a qualifier", string(q))
	}
}

func TestNewApproxDate_Bounds(t *testing.T) {
	t.Parallel()

	day, month := models.DatePrecisionDay, models.DatePrecisionMonth
	year, decade := models.DatePrecisionYear, models.DatePrecisionDecade
	exact, about := models.DateQualifierExact, models.DateQualifierAbout
	before, after, between := models.DateQualifierBefore, models.DateQualifierAfter, models.DateQualifierBetween

	// An empty earliest or latest is an open bound, the zero Date.
	cases := []struct {
		name      string
		qualifier models.DateQualifier
		precision models.DatePrecision
		anchors   []string
		earliest  string
		latest    string
	}{
		{"exact day", exact, day, []string{dayInMarch}, dayInMarch, dayInMarch},
		{"about a day", about, day, []string{dayInMarch}, "1998-03-13", "1998-03-15"},
		{"about a leap day", about, day, []string{leapDay}, "2000-02-28", "2000-03-01"},
		{"before a day", before, day, []string{dayInMarch}, "", dayInMarch},
		{"after a day", after, day, []string{dayInMarch}, dayInMarch, ""},
		{"between two days", between, day, []string{dayInMarch, "1998-03-20"}, dayInMarch, "1998-03-20"},

		{"exact month", exact, month, []string{dayInMarch}, "1998-03-01", "1998-03-31"},
		{"exact leap February", exact, month, []string{"2000-02-10"}, "2000-02-01", leapDay},
		{"about March 1998", about, month, []string{dayInMarch}, "1998-02-01", "1998-04-30"},
		{"about a January crosses the year", about, month, []string{"1998-01-20"}, "1997-12-01", "1998-02-28"},
		{"before a month", before, month, []string{dayInMarch}, "", "1998-03-31"},
		{"after a month", after, month, []string{dayInMarch}, "1998-03-01", ""},
		{"between two months", between, month, []string{dayInMarch, "1999-02-10"}, "1998-03-01", "1999-02-28"},

		{"exact year", exact, year, []string{"1998-07-04"}, "1998-01-01", "1998-12-31"},
		{"about 1998", about, year, []string{"1998-07-04"}, "1997-01-01", end1990s},
		{"before 1998", before, year, []string{"1998-07-04"}, "", "1998-12-31"},
		{"after 1998", after, year, []string{"1998-07-04"}, "1998-01-01", ""},
		{"between 1995 and 1998", between, year, []string{"1995-06-01", "1998-07-04"}, "1995-01-01", "1998-12-31"},
		{"between two years in a row", between, year, []string{"1998-01-01", "1999-01-01"}, "1998-01-01", end1990s},

		{"exact decade", exact, decade, []string{"1994-05-05"}, start1990s, end1990s},
		{"about the 1990s", about, decade, []string{"1994-05-05"}, "1980-01-01", "2009-12-31"},
		{"before the 1990s", before, decade, []string{"1994-05-05"}, "", end1990s},
		{"after the 1990s", after, decade, []string{"1994-05-05"}, start1990s, ""},
		{"between two decades", between, decade, []string{"1983-01-01", "1994-05-05"}, "1980-01-01", end1990s},

		{"about the second year stays in range", about, year, []string{"0002-01-01"}, "0001-01-01", "0003-12-31"},
		{"about the year 9998 stays in range", about, year, []string{"9998-01-01"}, "9997-01-01", "9999-12-31"},
		{"the last decade", exact, decade, []string{"9999-12-31"}, "9990-01-01", "9999-12-31"},
		{"nothing known", exact, models.DatePrecisionUnknown, nil, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			anchors := make([]models.Date, 0, len(tc.anchors))
			for _, s := range tc.anchors {
				anchors = append(anchors, mustParseDate(t, s))
			}

			a, err := models.NewApproxDate(tc.qualifier, tc.precision, anchors...)
			require.NoError(t, err)
			assert.Equal(t, models.ApproxDate{
				Earliest:  boundOf(t, tc.earliest),
				Latest:    boundOf(t, tc.latest),
				Precision: tc.precision,
				Qualifier: tc.qualifier,
			}, a)
			require.NoError(t, a.Validate(), "a built value is valid")
		})
	}
}

func TestNewApproxDate_RefusesInvalidShapes(t *testing.T) {
	t.Parallel()

	day, month := models.DatePrecisionDay, models.DatePrecisionMonth
	year, decade := models.DatePrecisionYear, models.DatePrecisionDecade
	exact, about := models.DateQualifierExact, models.DateQualifierAbout
	before, after, between := models.DateQualifierBefore, models.DateQualifierAfter, models.DateQualifierBetween
	on := func(s string) models.Date { return mustParseDate(t, s) }

	cases := []struct {
		name      string
		qualifier models.DateQualifier
		precision models.DatePrecision
		anchors   []models.Date
	}{
		{"a precision that is not one", exact, "week", []models.Date{on(dayInMarch)}},
		{"no precision", exact, "", []models.Date{on(dayInMarch)}},
		{"a qualifier that is not one", "circa", year, []models.Date{on(dayInMarch)}},
		{"no qualifier", "", year, []models.Date{on(dayInMarch)}},
		{"unknown with another qualifier", about, models.DatePrecisionUnknown, nil},
		{"unknown with an anchor", exact, models.DatePrecisionUnknown, []models.Date{on(dayInMarch)}},

		{"exact with no anchor", exact, year, nil},
		{"exact with two anchors", exact, year, []models.Date{on("1995-01-01"), on("1998-01-01")}},
		{"about with no anchor", about, year, nil},
		{"before with two anchors", before, year, []models.Date{on("1995-01-01"), on("1998-01-01")}},
		{"after with two anchors", after, year, []models.Date{on("1995-01-01"), on("1998-01-01")}},
		{"between with one anchor", between, year, []models.Date{on("1995-01-01")}},
		{"between with three anchors", between, year, []models.Date{on("1991-01-01"), on("1995-01-01"), on("1998-01-01")}},

		{"between whose second period comes first", between, year, []models.Date{on("1998-01-01"), on("1995-01-01")}},
		{"between the same day twice", between, day, []models.Date{on("1985-11-25"), on("1985-11-25")}},
		{"between two days of one year", between, year, []models.Date{on("1998-01-01"), on("1998-12-31")}},
		{"between two days of one month", between, month, []models.Date{on("1998-03-01"), on("1998-03-31")}},
		{"between two years of one decade", between, decade, []models.Date{on("1991-01-01"), on("1998-01-01")}},

		{"an anchor that is not a date", exact, day, []models.Date{{Year: 2026, Month: 2, Day: 30}}},
		{"the zero date as an anchor", exact, year, []models.Date{{}}},
		{"a second anchor that is not a date", between, day, []models.Date{on("2026-02-01"), {Year: 2026, Month: 2, Day: 30}}},

		{"about the first day", about, day, []models.Date{on("0001-01-01")}},
		{"about the last day", about, day, []models.Date{on("9999-12-31")}},
		{"about the first month", about, month, []models.Date{on("0001-01-15")}},
		{"about the last month", about, month, []models.Date{on("9999-12-15")}},
		{"about the year 1", about, year, []models.Date{on("0001-06-01")}},
		{"about the year 9999", about, year, []models.Date{on("9999-01-01")}},
		{"about the 0010s", about, decade, []models.Date{on("0012-01-01")}},
		{"about the 9990s", about, decade, []models.Date{on("9995-01-01")}},
		{"the decade of the years 1 to 9", exact, decade, []models.Date{on("0005-06-07")}},
		{"before the decade of the years 1 to 9", before, decade, []models.Date{on("0005-06-07")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, err := models.NewApproxDate(tc.qualifier, tc.precision, tc.anchors...)
			assert.Equal(t, models.ApproxDate{}, a)
			require.ErrorIs(t, err, models.ErrInvalidApproxDate)
			require.EqualError(t, err, models.ErrInvalidApproxDate.Error(), "the error text is fixed and never holds a date")
		})
	}
}

// edtfForm is one accepted EDTF text and the value it is. An empty earliest or
// latest is an open bound.
type edtfForm struct {
	text      string
	earliest  string
	latest    string
	precision models.DatePrecision
	qualifier models.DateQualifier
}

// edtfForms returns every accepted form, one per qualifier at each precision
// that has one.
func edtfForms() []edtfForm {
	day, month := models.DatePrecisionDay, models.DatePrecisionMonth
	year, decade := models.DatePrecisionYear, models.DatePrecisionDecade
	exact, about := models.DateQualifierExact, models.DateQualifierAbout
	before, after, between := models.DateQualifierBefore, models.DateQualifierAfter, models.DateQualifierBetween

	return []edtfForm{
		{dayInMarch, dayInMarch, dayInMarch, day, exact},
		{"1998-03", "1998-03-01", "1998-03-31", month, exact},
		{"1998", "1998-01-01", "1998-12-31", year, exact},
		{"199X", start1990s, end1990s, decade, exact},
		{"1998~", "1997-01-01", end1990s, year, about},
		{"1998-03~", "1998-02-01", "1998-04-30", month, about},
		{"1998-03-14~", "1998-03-13", "1998-03-15", day, about},
		{"199X~", "1980-01-01", "2009-12-31", decade, about},
		{"[..1998]", "", "1998-12-31", year, before},
		{"[..1998-03-14]", "", dayInMarch, day, before},
		{"[..199X]", "", end1990s, decade, before},
		{"[1998..]", "1998-01-01", "", year, after},
		{"[1995..1998]", "1995-01-01", "1998-12-31", year, between},
		{"[1998-03..1999-02]", "1998-03-01", "1999-02-28", month, between},
		{"[1984-11-26..1985-11-25]", "1984-11-26", "1985-11-25", day, between},
		{"[198X..199X]", "1980-01-01", end1990s, decade, between},
		{"XXXX", "", "", models.DatePrecisionUnknown, exact},
	}
}

// mustParseEDTF parses s, failing the test unless it is an accepted form.
func mustParseEDTF(t *testing.T, s string) models.ApproxDate {
	t.Helper()

	a, err := models.ParseEDTF(s)
	require.NoError(t, err)

	return a
}

func TestApproxDate_EDTFRoundTrip(t *testing.T) {
	t.Parallel()

	for _, form := range edtfForms() {
		t.Run(form.text, func(t *testing.T) {
			t.Parallel()

			a, err := models.ParseEDTF(form.text)
			require.NoError(t, err)
			assert.Equal(t, boundOf(t, form.earliest), a.Earliest, "earliest")
			assert.Equal(t, boundOf(t, form.latest), a.Latest, "latest")
			assert.Equal(t, form.precision, a.Precision, "precision")
			assert.Equal(t, form.qualifier, a.Qualifier, "qualifier")
			require.NoError(t, a.Validate())

			text, err := a.EDTF()
			require.NoError(t, err)
			assert.Equal(t, form.text, text, "the value writes back as its text")
		})
	}
}

func TestParseEDTF_RefusesOtherForms(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"1998?",                    // uncertain
		"1998%",                    // uncertain and approximate
		"1998-21",                  // a season
		"1995/1998",                // an interval is a span of time, not one day in it
		"[1667,1668]",              // one of a set of members
		"{1667..1668}",             // all of a set
		"Y17000",                   // a long year
		"-0100",                    // a negative year
		"19XX",                     // a century
		"1998-XX",                  // an unspecified month
		"1987-09-XX",               // an unspecified day is the month, written 1987-09
		"199x",                     // a lowercase unspecified digit
		"9/XX/1987",                // a date as a form shows it, not EDTF
		"[1998..1995]",             // the second period comes first
		"[1998..1998]",             // one period twice
		"[1985-11-25..1985-11-25]", // one day twice
		"[1998-03..1999]",          // two precisions
		"[..]",                     // no period
		"[..1998",                  // unclosed
		"1998~~",                   // approximate twice
		"[..1998]~",                // a set is not approximate
		"XXXX~",                    // nothing known is not approximate
		"0000",                     // the year 0
		"000X",                     // the decade of the year 0
		"+1998",                    // a sign
		"1998-3",                   // a one-digit month
		"1998-03-1",                // a one-digit day
		"1998-02-30",               // a day that does not exist
		"xxxx",                     // a lowercase unknown
		" 1998",                    // surrounding space
		"1998 ",                    // surrounding space
		"",                         // nothing
		"\xff",                     // not UTF-8
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			a, err := models.ParseEDTF(in)
			assert.Equal(t, models.ApproxDate{}, a)
			require.ErrorIs(t, err, models.ErrInvalidApproxDate)
			require.EqualError(t, err, models.ErrInvalidApproxDate.Error(), "the error text is fixed and never holds the text")
		})
	}
}

func TestApproxDate_ValidateRefusesInconsistentFields(t *testing.T) {
	t.Parallel()

	on := func(s string) models.Date { return mustParseDate(t, s) }
	year := models.DatePrecisionYear
	exact, about := models.DateQualifierExact, models.DateQualifierAbout

	for name, a := range map[string]models.ApproxDate{
		"the zero value":                     {},
		"a latest bound before the earliest": {Earliest: on("1999-01-01"), Latest: on("1998-12-31"), Precision: year, Qualifier: exact},
		"an earliest bound inside a period":  {Earliest: on("1998-01-02"), Latest: on("1998-12-31"), Precision: year, Qualifier: exact},
		"a latest bound inside a period":     {Earliest: on("1998-01-01"), Latest: on("1998-12-30"), Precision: year, Qualifier: exact},
		"an exact value over two periods":    {Earliest: on("1998-01-01"), Latest: on(end1990s), Precision: year, Qualifier: exact},
		"an exact value with open bounds":    {Precision: year, Qualifier: exact},
		"an about that is not widened":       {Earliest: on("1998-01-01"), Latest: on("1998-12-31"), Precision: year, Qualifier: about},
		"an about widened on one side":       {Earliest: on("1997-01-01"), Latest: on("1998-12-31"), Precision: year, Qualifier: about},
		"an about widened too far":           {Earliest: on("1996-01-01"), Latest: on(end1990s), Precision: year, Qualifier: about},
		"a before with an earliest bound": {
			Earliest: on(start1990s), Latest: on("1998-12-31"), Precision: year, Qualifier: models.DateQualifierBefore,
		},
		"an after with a latest bound": {
			Earliest: on("1998-01-01"), Latest: on(end1990s), Precision: year, Qualifier: models.DateQualifierAfter,
		},
		"a between inside one period": {
			Earliest: on("1998-01-01"), Latest: on("1998-12-31"), Precision: year, Qualifier: models.DateQualifierBetween,
		},
		"an unknown with bounds": {
			Earliest: on("1998-01-01"), Latest: on("1998-12-31"), Precision: models.DatePrecisionUnknown, Qualifier: exact,
		},
		"an unknown that is about":    {Precision: models.DatePrecisionUnknown, Qualifier: about},
		"a precision that is not one": {Earliest: on("1998-01-01"), Latest: on("1998-12-31"), Precision: "week", Qualifier: exact},
		"a qualifier that is not one": {Earliest: on("1998-01-01"), Latest: on("1998-12-31"), Precision: year, Qualifier: "circa"},
		"a bound that is not a date": {
			Earliest: models.Date{Year: 2026, Month: 2, Day: 30}, Latest: models.Date{Year: 2026, Month: 2, Day: 30},
			Precision: models.DatePrecisionDay, Qualifier: exact,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, a.Validate(), models.ErrInvalidApproxDate)

			text, err := a.EDTF()
			require.ErrorIs(t, err, models.ErrInvalidApproxDate)
			assert.Empty(t, text)

			v, err := a.Value()
			require.ErrorIs(t, err, models.ErrInvalidApproxDate)
			assert.Nil(t, v)

			b, err := a.MarshalText()
			require.ErrorIs(t, err, models.ErrInvalidApproxDate)
			assert.Nil(t, b)
		})
	}
}

func TestApproxDate_Period(t *testing.T) {
	t.Parallel()

	day, month := models.DatePrecisionDay, models.DatePrecisionMonth
	year, decade := models.DatePrecisionYear, models.DatePrecisionDecade

	cases := []struct {
		name      string
		text      string
		first     string
		last      string
		precision models.DatePrecision
	}{
		{"an exact day is that day", dayInMarch, dayInMarch, dayInMarch, day},
		{"an exact month is that month", "1998-03", "1998-03-01", "1998-03-31", month},
		{"an exact 1998 is 1998", "1998", "1998-01-01", "1998-12-31", year},
		{"an exact decade is that decade", "199X", start1990s, end1990s, decade},
		{"about a day is its month", "1998-03-14~", "1998-03-01", "1998-03-31", month},
		{"about March 1998 is 1998", "1998-03~", "1998-01-01", "1998-12-31", year},
		{"about 1998 is the 1990s", "1998~", start1990s, end1990s, decade},
		{"between 1995 and 1998 is the 1990s", "[1995..1998]", start1990s, end1990s, decade},
		{"between two months is the 1990s", "[1998-03..1999-02]", start1990s, end1990s, decade},
		{"the days an age allows are the 1980s", "[1984-11-26..1985-11-25]", "1980-01-01", "1989-12-31", decade},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			first, last, p, ok := mustParseEDTF(t, tc.text).Period()
			require.True(t, ok)
			assert.Equal(t, mustParseDate(t, tc.first), first)
			assert.Equal(t, mustParseDate(t, tc.last), last)
			assert.Equal(t, tc.precision, p)
		})
	}

	none := map[string]models.ApproxDate{
		"about 1990 spans two decades":     mustParseEDTF(t, "1990~"),
		"about the 1990s spans three":      mustParseEDTF(t, "199X~"),
		"between two decades spans two":    mustParseEDTF(t, "[198X..199X]"),
		"a range across a decade's end":    mustParseEDTF(t, "[1999-03-01..2000-02-29]"),
		"before has no first day":          mustParseEDTF(t, "[..1998]"),
		"after has no last day":            mustParseEDTF(t, "[1998..]"),
		"nothing known names no period":    mustParseEDTF(t, "XXXX"),
		"the zero value is not a date":     {},
		"an inconsistent value is refused": {Earliest: mustParseDate(t, "1998-01-02"), Latest: mustParseDate(t, "1998-12-31"), Precision: year, Qualifier: models.DateQualifierExact},
	}

	for name, a := range none {
		first, last, p, ok := a.Period()
		assert.False(t, ok, name)
		assert.Equal(t, models.Date{}, first, name)
		assert.Equal(t, models.Date{}, last, name)
		assert.Empty(t, p, name)
	}
}

func TestApproxDate_TextAndJSON(t *testing.T) {
	t.Parallel()

	type record struct {
		When  models.ApproxDate  `json:"when"`
		Maybe *models.ApproxDate `json:"maybe"`
		Skip  models.ApproxDate  `json:"skip,omitzero"`
	}

	about := mustParseEDTF(t, "1998~")

	encoded, err := json.Marshal(record{When: about})
	require.NoError(t, err)
	assert.JSONEq(t, `{"when":"1998~","maybe":null}`, string(encoded))
	assert.NotContains(t, string(encoded), "skip", "omitzero omits the zero value")

	ageRange := mustParseEDTF(t, "[1984-11-26..1985-11-25]")
	want := record{When: mustParseEDTF(t, "XXXX"), Maybe: &ageRange}

	encoded, err = json.Marshal(want)
	require.NoError(t, err)
	assert.JSONEq(t, `{"when":"XXXX","maybe":"[1984-11-26..1985-11-25]"}`, string(encoded))

	var decoded record
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, want, decoded)

	_, err = json.Marshal(record{})
	require.ErrorIs(t, err, models.ErrInvalidApproxDate, "the zero value is not a date")

	err = json.Unmarshal([]byte(`{"when":"1995/1998"}`), &decoded)
	require.ErrorIs(t, err, models.ErrInvalidApproxDate)
	assert.NotContains(t, err.Error(), "1995/1998", "the error never repeats the text")

	text, err := about.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "1998~", string(text))

	var back models.ApproxDate
	require.NoError(t, back.UnmarshalText(text))
	assert.Equal(t, about, back)

	require.ErrorIs(t, back.UnmarshalText([]byte("1998?")), models.ErrInvalidApproxDate)
	assert.Equal(t, about, back, "a failed decode leaves the value unchanged")
}

func TestApproxDate_ValueAndScan(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "text", models.ApproxDate{}.GormDataType())

	v, err := mustParseEDTF(t, "[1995..1998]").Value()
	require.NoError(t, err)
	assert.Equal(t, "[1995..1998]", v, "the driver value is the EDTF string")

	v, err = models.ApproxDate{}.Value()
	require.ErrorIs(t, err, models.ErrInvalidApproxDate, "the zero value is not a date")
	assert.Nil(t, v)

	want := mustParseEDTF(t, "1998~")

	for name, src := range map[string]any{
		"string": "1998~",
		"bytes":  []byte("1998~"),
	} {
		var got models.ApproxDate
		require.NoError(t, got.Scan(src), name)
		assert.Equal(t, want, got, name)
	}

	preset := mustParseEDTF(t, "[..199X]")

	for name, src := range map[string]any{
		"nil":         nil,
		"number":      int64(1998),
		"an interval": "1998/1999",
		"empty bytes": []byte{},
		"a time":      time.Date(1998, time.March, 14, 0, 0, 0, 0, time.UTC),
	} {
		got := preset
		require.ErrorIs(t, got.Scan(src), models.ErrInvalidApproxDate, name)
		assert.Equal(t, preset, got, "%s: a failed scan leaves the value unchanged", name)
	}
}

// approxRow is a model with a required and an optional approximate date.
type approxRow struct {
	ID    uint
	EDTF  models.ApproxDate `gorm:"column:edtf;not null"`
	Maybe *models.ApproxDate
}

// newApproxDB opens an in-memory SQLite database holding approx_rows.
func newApproxDB(t *testing.T) *gorm.DB {
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

	require.NoError(t, db.AutoMigrate(&approxRow{}))

	return db
}

func TestApproxDate_SQLiteRoundTrip(t *testing.T) {
	t.Parallel()

	db := newApproxDB(t)

	var ddl string
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'approx_rows'").Scan(&ddl).Error)
	assert.Contains(t, ddl, "`edtf` text NOT NULL")
	assert.Contains(t, ddl, "`maybe` text")

	forms := edtfForms()
	texts := make([]string, 0, len(forms))
	written := make([]approxRow, 0, len(forms))

	for _, form := range forms {
		texts = append(texts, form.text)
		written = append(written, approxRow{EDTF: mustParseEDTF(t, form.text)})
	}

	maybe := mustParseEDTF(t, "[1984-11-26..1985-11-25]")
	written[0].Maybe = &maybe

	for i := range written {
		require.NoError(t, db.Create(&written[i]).Error)
	}

	var read []approxRow
	require.NoError(t, db.Order("id").Find(&read).Error)
	assert.Equal(t, written, read, "every form reads back equal, and a nil pointer reads back nil")

	var stored []string
	require.NoError(t, db.Raw("SELECT edtf FROM approx_rows ORDER BY id").Scan(&stored).Error)
	assert.Equal(t, texts, stored, "the column holds the EDTF text")

	err := db.Create(&approxRow{}).Error
	require.ErrorIs(t, err, models.ErrInvalidApproxDate)

	var count int64
	require.NoError(t, db.Model(&approxRow{}).Count(&count).Error)
	assert.Equal(t, int64(len(written)), count, "a refused value writes no row")
}

// FuzzParseEDTF proves ParseEDTF never panics, refuses with exactly
// ErrInvalidApproxDate and the zero value, and accepts only text that its value
// writes back unchanged.
func FuzzParseEDTF(f *testing.F) {
	for _, seed := range []string{
		"1998",
		"199X",
		"1998~",
		"[..1998]",
		"[1995..1998]",
		"[1984-11-26..1985-11-25]",
		"XXXX",
		"1998/1999",
		"",
		"\xff",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		a, err := models.ParseEDTF(in)
		if err != nil {
			if !errors.Is(err, models.ErrInvalidApproxDate) || err.Error() != models.ErrInvalidApproxDate.Error() ||
				a != (models.ApproxDate{}) {
				t.Fatalf("ParseEDTF refused with %v and %+v", err, a)
			}

			return
		}

		checkParsedApproxDate(t, in, a)
	})
}

// checkParsedApproxDate fails t unless a is valid, writes back exactly as in,
// and has its earliest bound no later than its latest.
func checkParsedApproxDate(t *testing.T, in string, a models.ApproxDate) {
	t.Helper()

	if err := a.Validate(); err != nil {
		t.Fatalf("ParseEDTF(%q).Validate() = %v", in, err)
	}

	if text, err := a.EDTF(); err != nil || text != in {
		t.Fatalf("ParseEDTF(%q).EDTF() = %q, %v", in, text, err)
	}

	if !a.Earliest.IsZero() && !a.Latest.IsZero() && a.Earliest.After(a.Latest) {
		t.Fatalf("ParseEDTF(%q) has its earliest bound after its latest: %+v", in, a)
	}
}

func TestApproxDate_HoldsPartialDatesAndAgeRanges(t *testing.T) {
	t.Parallel()

	day, month, year := models.DatePrecisionDay, models.DatePrecisionMonth, models.DatePrecisionYear
	exact, between := models.DateQualifierExact, models.DateQualifierBetween

	// Each shape is built from the days it names. Its period is the narrowest
	// one Period gives (empty for none).
	cases := []struct {
		name        string
		qualifier   models.DateQualifier
		precision   models.DatePrecision
		anchors     []string
		earliest    string
		latest      string
		text        string
		periodFirst string
		periodLast  string
	}{
		{
			// Written 6/1985: any day of the month builds it.
			name: "a month and year", qualifier: exact, precision: month, anchors: []string{"1985-06-17"},
			earliest: "1985-06-01", latest: "1985-06-30", text: "1985-06",
			periodFirst: "1985-06-01", periodLast: "1985-06-30",
		},
		{
			// Written 9/XX/1987: the masked day is never stored as a day.
			name: "a month with its day masked", qualifier: exact, precision: month, anchors: []string{"1987-09-01"},
			earliest: "1987-09-01", latest: "1987-09-30", text: "1987-09",
			periodFirst: "1987-09-01", periodLast: "1987-09-30",
		},
		{
			name: "a year alone", qualifier: exact, precision: year, anchors: []string{"1957-01-01"},
			earliest: "1957-01-01", latest: "1957-12-31", text: "1957",
			periodFirst: "1957-01-01", periodLast: "1957-12-31",
		},
		{
			// Someone 41 years old on 2026-11-25 was born on or after 1984-11-26
			// and on or before 1985-11-25.
			name: "the days an age allows on a known day", qualifier: between, precision: day,
			anchors:  []string{"1984-11-26", "1985-11-25"},
			earliest: "1984-11-26", latest: "1985-11-25", text: "[1984-11-26..1985-11-25]",
			periodFirst: "1980-01-01", periodLast: "1989-12-31",
		},
		{
			// It crosses the end of a decade, so no single period holds it.
			name: "a range ending on a leap day", qualifier: between, precision: day,
			anchors:  []string{"1999-03-01", leapDay},
			earliest: "1999-03-01", latest: leapDay, text: "[1999-03-01..2000-02-29]",
		},
		{
			// Written 9/2025: a death known to its month, built from its last day.
			name: "the month of a death", qualifier: exact, precision: month, anchors: []string{"2025-09-30"},
			earliest: "2025-09-01", latest: "2025-09-30", text: "2025-09",
			periodFirst: "2025-09-01", periodLast: "2025-09-30",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			anchors := make([]models.Date, 0, len(tc.anchors))
			for _, s := range tc.anchors {
				anchors = append(anchors, mustParseDate(t, s))
			}

			a, err := models.NewApproxDate(tc.qualifier, tc.precision, anchors...)
			require.NoError(t, err)
			assert.Equal(t, mustParseDate(t, tc.earliest), a.Earliest, "earliest")
			assert.Equal(t, mustParseDate(t, tc.latest), a.Latest, "latest")

			text, err := a.EDTF()
			require.NoError(t, err)
			assert.Equal(t, tc.text, text)
			assert.Equal(t, a, mustParseEDTF(t, text), "its text parses back to it")

			first, last, _, ok := a.Period()
			assert.Equal(t, tc.periodFirst != "", ok, "whether one period holds it")
			assert.Equal(t, boundOf(t, tc.periodFirst), first)
			assert.Equal(t, boundOf(t, tc.periodLast), last)

			type record struct {
				When models.ApproxDate `json:"when"`
			}

			encoded, err := json.Marshal(record{When: a})
			require.NoError(t, err)
			assert.JSONEq(t, `{"when":"`+tc.text+`"}`, string(encoded))

			var decoded record
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, a, decoded.When, "it survives JSON")

			db := newApproxDB(t)
			row := approxRow{EDTF: a}
			require.NoError(t, db.Create(&row).Error)

			var read approxRow
			require.NoError(t, db.First(&read, row.ID).Error)
			assert.Equal(t, a, read.EDTF, "it survives a text column")
		})
	}
}
