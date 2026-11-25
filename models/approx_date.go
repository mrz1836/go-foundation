package models

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidApproxDate indicates a value that is not a valid approximate date,
// or text that is not one of the EDTF forms ParseEDTF accepts. Its text is
// fixed and never includes a date or the text.
var ErrInvalidApproxDate = errors.New("invalid approximate date")

// The lengths of a month's text (YYYY-MM) and of a year's or a decade's (YYYY,
// YYYX); a day's is dateTextLength.
const (
	monthTextLength = len("2006-01")
	yearTextLength  = len("2006")
)

// yearsPerDecade is the length of a decade, which starts in a year ending in 0.
const yearsPerDecade = 10

// The marks of the EDTF forms ApproxDate writes and reads.
const (
	edtfUnknown     = "XXXX" // nothing is known
	edtfAbout       = "~"    // approximate
	edtfSetOpen     = "["    // opens one day of a set: before, after, or between
	edtfSetClose    = "]"    // closes it
	edtfRange       = ".."   // the days from one period through another, open on an empty side
	edtfDecadeDigit = 'X'    // the unspecified last digit of a decade's year
)

// DatePrecision is how much of a date is known.
type DatePrecision string

// The DatePrecision values.
const (
	DatePrecisionDay     DatePrecision = "day"     // the day is known
	DatePrecisionMonth   DatePrecision = "month"   // only the month
	DatePrecisionYear    DatePrecision = "year"    // only the year
	DatePrecisionDecade  DatePrecision = "decade"  // only the decade
	DatePrecisionUnknown DatePrecision = "unknown" // nothing is known
)

// Valid reports whether p is one of the DatePrecision values.
func (p DatePrecision) Valid() bool {
	switch p {
	case DatePrecisionDay, DatePrecisionMonth, DatePrecisionYear, DatePrecisionDecade, DatePrecisionUnknown:
		return true
	default:
		return false
	}
}

// PeriodOf returns the first and last days of the period of precision p that
// holds d: d itself for a day; the first and last days of d's month or year;
// and, for a decade, January 1 of the year ending in 0 at or before d's year
// through December 31 nine years later. ok is false, with zero dates, for
// DatePrecisionUnknown, a precision that is not one of the values, a d that is
// not a real calendar date, and a period that would begin before 0001-01-01
// (the decade of the years 1 through 9 would start in the year 0).
func (p DatePrecision) PeriodOf(d Date) (first, last Date, ok bool) {
	if !d.valid() {
		return Date{}, Date{}, false
	}

	switch p {
	case DatePrecisionDay:
		first, last = d, d
	case DatePrecisionMonth:
		first = Date{Year: d.Year, Month: d.Month, Day: 1}
		last = DateOf(time.Date(d.Year, d.Month+1, 0, 0, 0, 0, 0, time.UTC))
	case DatePrecisionYear:
		first = Date{Year: d.Year, Month: time.January, Day: 1}
		last = Date{Year: d.Year, Month: time.December, Day: 31}
	case DatePrecisionDecade:
		start := d.Year - d.Year%yearsPerDecade
		first = Date{Year: start, Month: time.January, Day: 1}
		last = Date{Year: start + yearsPerDecade - 1, Month: time.December, Day: 31}
	case DatePrecisionUnknown:
		return Date{}, Date{}, false
	default:
		return Date{}, Date{}, false
	}

	if !first.valid() {
		return Date{}, Date{}, false
	}

	return first, last, true
}

// DateQualifier says how an approximate date relates to the period it names.
type DateQualifier string

// The DateQualifier values.
const (
	DateQualifierExact   DateQualifier = "exact"   // some day in the period
	DateQualifierAbout   DateQualifier = "about"   // near it: one unit of the precision either side
	DateQualifierBefore  DateQualifier = "before"  // on or before the period's last day
	DateQualifierAfter   DateQualifier = "after"   // on or after the period's first day
	DateQualifierBetween DateQualifier = "between" // from one period's first day to a later one's last
)

// Valid reports whether q is one of the DateQualifier values.
func (q DateQualifier) Valid() bool {
	switch q {
	case DateQualifierExact, DateQualifierAbout, DateQualifierBefore, DateQualifierAfter, DateQualifierBetween:
		return true
	default:
		return false
	}
}

// ApproxDate is a date known only to a day, a month, a year, or a decade,
// possibly qualified as about, before, after, or between: the first and last
// days it can be (the zero Date when a bound is open), its precision, and its
// qualifier. Its one text form is EDTF (ISO 8601-2), which ParseEDTF reads and
// EDTF writes; JSON and a SQL text column use it.
//
// About widens the period by one unit of the precision on each side, so about
// 1998 is 1997-01-01 through 1999-12-31; the qualifier is kept, so a reader can
// recover the period itself. Before and after are inclusive: before 1998 is any
// day through 1998-12-31, and after 1998 any day from 1998-01-01.
//
// A value from NewApproxDate or ParseEDTF is always valid. The fields are
// exported, so a value built by hand can be inconsistent; Validate, EDTF, Value,
// and MarshalText refuse it. The zero ApproxDate is not a date, so store an
// optional one as *ApproxDate.
//
//nolint:recvcheck // Scan and UnmarshalText set the value through a pointer; the other methods take a value, as database/sql and encoding expect
type ApproxDate struct {
	Earliest  Date          // the first day it can be; the zero Date when open (before, unknown)
	Latest    Date          // the last day it can be; the zero Date when open (after, unknown)
	Precision DatePrecision // how much of the date is known
	Qualifier DateQualifier // how it relates to the period it names
}

// NewApproxDate returns the approximate date q and p make of anchors, days that
// name its periods: any day of a period names it. DatePrecisionUnknown takes
// DateQualifierExact and no anchor, and has both bounds open. Every other
// precision takes one anchor, or two for DateQualifierBetween, whose second
// period must start after its first one ends. With P the anchor's period of
// precision p: exact is P; about is P widened by one unit of p (a day, a month,
// a year, or ten years) on each side; before runs from an open bound through
// P's last day; after from P's first day to an open bound; and between from the
// first period's first day through the second's last. Anything else, a bound
// outside 0001-01-01 through 9999-12-31 included, returns the zero ApproxDate
// and ErrInvalidApproxDate.
func NewApproxDate(q DateQualifier, p DatePrecision, anchors ...Date) (ApproxDate, error) {
	if !q.Valid() || !p.Valid() {
		return ApproxDate{}, ErrInvalidApproxDate
	}

	if p == DatePrecisionUnknown {
		if q != DateQualifierExact || len(anchors) != 0 {
			return ApproxDate{}, ErrInvalidApproxDate
		}

		return ApproxDate{Precision: p, Qualifier: q}, nil
	}

	earliest, latest, ok := approxBounds(q, p, anchors)
	if !ok {
		return ApproxDate{}, ErrInvalidApproxDate
	}

	return ApproxDate{Earliest: earliest, Latest: latest, Precision: p, Qualifier: q}, nil
}

// ParseEDTF parses s, which must be exactly one of the EDTF forms EDTF writes:
// a period, written YYYY-MM-DD, YYYY-MM, YYYY, or YYYX for a decade (each year
// from 0001 through 9999), alone (exact) or followed by ~ (about); [..P]
// (before); [P..] (after); [P1..P2] (between, both periods of one precision);
// or XXXX (nothing known). Anything else (other EDTF features such as intervals,
// uncertainty, seasons, and unspecified months or days, a surrounding space, and
// a lowercase x) returns the zero ApproxDate and ErrInvalidApproxDate, whose
// text never includes s. A parsed value's EDTF is s exactly.
func ParseEDTF(s string) (ApproxDate, error) {
	a, ok := parseEDTF(s)
	if !ok {
		return ApproxDate{}, ErrInvalidApproxDate
	}

	if text, err := a.EDTF(); err != nil || text != s {
		return ApproxDate{}, ErrInvalidApproxDate
	}

	return a, nil
}

// EDTF returns a's one EDTF text: its period, written YYYY-MM-DD, YYYY-MM, YYYY,
// or YYYX for a decade, alone (exact) or followed by ~ (about); [..P] (before);
// [P..] (after); [P1..P2] (between); or XXXX (nothing known). It returns "" and
// ErrInvalidApproxDate for a value Validate refuses.
func (a ApproxDate) EDTF() (string, error) {
	anchors, ok := a.anchors()
	if !ok {
		return "", ErrInvalidApproxDate
	}

	return a.text(anchors), nil
}

// Validate returns nil when a is a value NewApproxDate could have built: its
// precision and qualifier are values of their types, and its bounds are exactly
// the ones they give its periods. Otherwise it returns ErrInvalidApproxDate.
func (a ApproxDate) Validate() error {
	if _, ok := a.anchors(); !ok {
		return ErrInvalidApproxDate
	}

	return nil
}

// Period returns the narrowest single day, month, year, or decade, no finer
// than a's precision, that holds both of a's bounds: an exact 1998 is 1998, and
// about 1998 and between 1995 and 1998 are both the 1990s. ok is false, with
// zero values, when Validate refuses a, when a bound is open (before, after,
// and nothing known), and when no decade holds both bounds, as for about 1990.
func (a ApproxDate) Period() (first, last Date, p DatePrecision, ok bool) {
	if a.Validate() != nil || a.Earliest.IsZero() || a.Latest.IsZero() {
		return Date{}, Date{}, "", false
	}

	for wider := a.Precision; wider != ""; wider = wider.coarser() {
		start, end, found := wider.PeriodOf(a.Earliest)
		if found && !end.Before(a.Latest) {
			return start, end, wider, true
		}
	}

	return Date{}, Date{}, "", false
}

// Value implements driver.Valuer. It writes a as its EDTF text, for a text
// column, and returns ErrInvalidApproxDate for a value Validate refuses, the
// zero ApproxDate included.
func (a ApproxDate) Value() (driver.Value, error) {
	text, err := a.EDTF()
	if err != nil {
		return nil, err
	}

	return text, nil
}

// Scan implements sql.Scanner. It reads a string or []byte with ParseEDTF.
// Anything else, nil included, and text ParseEDTF refuses, returns
// ErrInvalidApproxDate and leaves a unchanged. A NULL belongs in a
// *ApproxDate, which database/sql and GORM set to nil without calling Scan.
func (a *ApproxDate) Scan(src any) error {
	var text string

	switch v := src.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return ErrInvalidApproxDate
	}

	parsed, err := ParseEDTF(text)
	if err != nil {
		return err
	}

	*a = parsed

	return nil
}

// GormDataType returns "text", so GORM declares a text column for an
// ApproxDate field (and a nullable one for a *ApproxDate field).
func (ApproxDate) GormDataType() string {
	return "text"
}

// MarshalText implements encoding.TextMarshaler, so JSON writes an ApproxDate
// as its EDTF string. Like Value, it returns ErrInvalidApproxDate for a value
// Validate refuses, the zero ApproxDate included.
func (a ApproxDate) MarshalText() ([]byte, error) {
	text, err := a.EDTF()
	if err != nil {
		return nil, err
	}

	return []byte(text), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with ParseEDTF. On an
// error it returns ErrInvalidApproxDate and leaves a unchanged.
func (a *ApproxDate) UnmarshalText(text []byte) error {
	parsed, err := ParseEDTF(string(text))
	if err != nil {
		return err
	}

	*a = parsed

	return nil
}

// anchors recovers the days a names its periods by (none when nothing is
// known), and reports whether NewApproxDate builds exactly a from them.
func (a ApproxDate) anchors() ([]Date, bool) {
	var anchors []Date

	switch a.Qualifier {
	case DateQualifierExact, DateQualifierAfter:
		anchors = []Date{a.Earliest}
	case DateQualifierAbout:
		anchor, ok := a.Precision.step(a.Earliest, 1)
		if !ok {
			return nil, false
		}

		anchors = []Date{anchor}
	case DateQualifierBefore:
		anchors = []Date{a.Latest}
	case DateQualifierBetween:
		anchors = []Date{a.Earliest, a.Latest}
	default:
		return nil, false
	}

	if a.Precision == DatePrecisionUnknown {
		anchors = nil
	}

	rebuilt, err := NewApproxDate(a.Qualifier, a.Precision, anchors...)
	if err != nil || rebuilt != a {
		return nil, false
	}

	return anchors, true
}

// text writes a, a valid value, in EDTF from the anchors that build it.
func (a ApproxDate) text(anchors []Date) string {
	if a.Precision == DatePrecisionUnknown {
		return edtfUnknown
	}

	switch a.Qualifier {
	case DateQualifierExact:
		return a.Precision.periodText(anchors[0])
	case DateQualifierAbout:
		return a.Precision.periodText(anchors[0]) + edtfAbout
	case DateQualifierBefore:
		return edtfSetOpen + edtfRange + a.Precision.periodText(anchors[0]) + edtfSetClose
	case DateQualifierAfter:
		return edtfSetOpen + a.Precision.periodText(anchors[0]) + edtfRange + edtfSetClose
	case DateQualifierBetween:
		return edtfSetOpen + a.Precision.periodText(anchors[0]) + edtfRange + a.Precision.periodText(anchors[1]) + edtfSetClose
	default:
		return ""
	}
}

// step returns the first day of the period n units of p after the one that
// starts on d, and false when that day falls outside 0001-01-01 through
// 9999-12-31 or p names no period.
func (p DatePrecision) step(d Date, n int) (Date, bool) {
	var t time.Time

	switch p {
	case DatePrecisionDay:
		t = time.Date(d.Year, d.Month, d.Day+n, 0, 0, 0, 0, time.UTC)
	case DatePrecisionMonth:
		t = time.Date(d.Year, d.Month+time.Month(n), d.Day, 0, 0, 0, 0, time.UTC)
	case DatePrecisionYear:
		t = time.Date(d.Year+n, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	case DatePrecisionDecade:
		t = time.Date(d.Year+n*yearsPerDecade, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	case DatePrecisionUnknown:
		return Date{}, false
	default:
		return Date{}, false
	}

	moved := DateOf(t)

	return moved, moved.valid()
}

// coarser returns the next precision up from p: a month from a day, a year
// from a month, and a decade from a year. Above a decade, it returns "".
func (p DatePrecision) coarser() DatePrecision {
	switch p {
	case DatePrecisionDay:
		return DatePrecisionMonth
	case DatePrecisionMonth:
		return DatePrecisionYear
	case DatePrecisionYear:
		return DatePrecisionDecade
	case DatePrecisionDecade, DatePrecisionUnknown:
		return ""
	default:
		return ""
	}
}

// periodText writes the period of precision p that holds d: YYYY-MM-DD,
// YYYY-MM, YYYY, or YYYX for a decade.
func (p DatePrecision) periodText(d Date) string {
	switch p {
	case DatePrecisionDay:
		return d.String()
	case DatePrecisionMonth:
		return fmt.Sprintf("%04d-%02d", d.Year, int(d.Month))
	case DatePrecisionYear:
		return fmt.Sprintf("%04d", d.Year)
	case DatePrecisionDecade:
		return fmt.Sprintf("%03d%c", d.Year/yearsPerDecade, edtfDecadeDigit)
	case DatePrecisionUnknown:
		return edtfUnknown
	default:
		return ""
	}
}

// approxBounds returns the bounds q gives the periods of p that anchors name,
// for any precision but DatePrecisionUnknown.
func approxBounds(q DateQualifier, p DatePrecision, anchors []Date) (earliest, latest Date, ok bool) {
	want := 1
	if q == DateQualifierBetween {
		want = 2
	}

	if len(anchors) != want {
		return Date{}, Date{}, false
	}

	first, last, ok := p.PeriodOf(anchors[0])
	if !ok {
		return Date{}, Date{}, false
	}

	switch q {
	case DateQualifierExact:
		return first, last, true
	case DateQualifierAbout:
		return aboutBounds(p, first)
	case DateQualifierBefore:
		return Date{}, last, true
	case DateQualifierAfter:
		return first, Date{}, true
	case DateQualifierBetween:
		return betweenBounds(p, first, last, anchors[1])
	default:
		return Date{}, Date{}, false
	}
}

// aboutBounds widens the period of p that starts on first by one unit of p on
// each side.
func aboutBounds(p DatePrecision, first Date) (earliest, latest Date, ok bool) {
	earliest, ok = p.step(first, -1)
	if !ok {
		return Date{}, Date{}, false
	}

	next, ok := p.step(first, 1)
	if !ok {
		return Date{}, Date{}, false
	}

	_, latest, ok = p.PeriodOf(next)
	if !ok {
		return Date{}, Date{}, false
	}

	return earliest, latest, true
}

// betweenBounds runs from first, the first day of one period of p, through the
// last day of the period of p that holds second, which must start after last,
// the first period's last day.
func betweenBounds(p DatePrecision, first, last, second Date) (earliest, latest Date, ok bool) {
	secondFirst, secondLast, ok := p.PeriodOf(second)
	if !ok || !secondFirst.After(last) {
		return Date{}, Date{}, false
	}

	return first, secondLast, true
}

// parseEDTF reads s as one of the forms ParseEDTF accepts, building the value
// with NewApproxDate; ParseEDTF then checks that the value writes back as s.
func parseEDTF(s string) (ApproxDate, bool) {
	if s == edtfUnknown {
		return buildApproxDate(DateQualifierExact, DatePrecisionUnknown)
	}

	if set, ok := strings.CutPrefix(s, edtfSetOpen); ok {
		return parseEDTFSet(set)
	}

	if period, ok := strings.CutSuffix(s, edtfAbout); ok {
		return parseQualified(DateQualifierAbout, period)
	}

	return parseQualified(DateQualifierExact, s)
}

// parseEDTFSet reads what follows the [ of a before, after, or between form:
// ..P], P..], or P1..P2], both periods of one precision.
func parseEDTFSet(s string) (ApproxDate, bool) {
	s, ok := strings.CutSuffix(s, edtfSetClose)
	if !ok {
		return ApproxDate{}, false
	}

	from, to, ok := strings.Cut(s, edtfRange)
	if !ok {
		return ApproxDate{}, false
	}

	switch {
	case from == "":
		return parseQualified(DateQualifierBefore, to)
	case to == "":
		return parseQualified(DateQualifierAfter, from)
	}

	fromDay, fromPrecision, okFrom := parsePeriod(from)
	toDay, toPrecision, okTo := parsePeriod(to)

	if !okFrom || !okTo || fromPrecision != toPrecision {
		return ApproxDate{}, false
	}

	return buildApproxDate(DateQualifierBetween, fromPrecision, fromDay, toDay)
}

// parseQualified reads s as one period and builds the value q makes of it.
func parseQualified(q DateQualifier, s string) (ApproxDate, bool) {
	anchor, p, ok := parsePeriod(s)
	if !ok {
		return ApproxDate{}, false
	}

	return buildApproxDate(q, p, anchor)
}

// parsePeriod reads one period's text, YYYY-MM-DD, YYYY-MM, YYYY, or YYYX, and
// returns a day in it and its precision.
func parsePeriod(s string) (Date, DatePrecision, bool) {
	switch len(s) {
	case dateTextLength:
		d, err := ParseDate(s)

		return d, DatePrecisionDay, err == nil
	case monthTextLength:
		return parseMonth(s)
	case yearTextLength:
		return parseYearOrDecade(s)
	default:
		return Date{}, "", false
	}
}

// parseMonth reads YYYY-MM, a month from 01 through 12, as its first day.
func parseMonth(s string) (Date, DatePrecision, bool) {
	year, okYear := parseDigits(s[:yearTextLength])
	month, okMonth := parseDigits(s[yearTextLength+1:])

	if !okYear || !okMonth || s[yearTextLength] != '-' {
		return Date{}, "", false
	}

	d, err := NewDate(year, time.Month(month), 1)

	return d, DatePrecisionMonth, err == nil
}

// parseYearOrDecade reads YYYY as its year's first day, or YYYX as its
// decade's.
func parseYearOrDecade(s string) (Date, DatePrecision, bool) {
	p := DatePrecisionYear
	digits := s

	if s[len(s)-1] == edtfDecadeDigit {
		p, digits = DatePrecisionDecade, s[:len(s)-1]
	}

	n, ok := parseDigits(digits)
	if !ok {
		return Date{}, "", false
	}

	if p == DatePrecisionDecade {
		n *= yearsPerDecade
	}

	d, err := NewDate(n, time.January, 1)

	return d, p, err == nil
}

// parseDigits reads s, which must be one or more ASCII digits and nothing else
// (no sign, space, or other numeral).
func parseDigits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}

	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}

	n, err := strconv.Atoi(s)

	return n, err == nil
}

// buildApproxDate is NewApproxDate, reporting success as a bool.
func buildApproxDate(q DateQualifier, p DatePrecision, anchors ...Date) (ApproxDate, bool) {
	a, err := NewApproxDate(q, p, anchors...)

	return a, err == nil
}
