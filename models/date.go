package models

import (
	"cmp"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidDate indicates a value that is not a real calendar date written
// YYYY-MM-DD, from 0001-01-01 through 9999-12-31. Its text is fixed and never
// includes the value.
var ErrInvalidDate = errors.New("invalid calendar date")

// dateTextLength is the length of a date written YYYY-MM-DD.
const dateTextLength = len(time.DateOnly)

// Date is a calendar date (a year, a month, and a day) with no time of day and
// no time zone, from 0001-01-01 through 9999-12-31. It maps to a SQL date
// column and is written as YYYY-MM-DD text, so no database session time zone
// can move it; text and JSON use the same form. The zero Date is not a date:
// Value and MarshalText refuse it, so store an optional date as *Date.
//
// A Date from NewDate, ParseDate, or an in-range DateOf is always valid. The
// fields are exported, so a Date built by hand can name a day that doesn't
// exist; Value and MarshalText refuse it too.
//
//nolint:recvcheck // Scan and UnmarshalText set the value through a pointer; the other methods take a value, as database/sql and encoding expect
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate returns the Date of year, month, and day, or the zero Date and
// ErrInvalidDate unless they name a real calendar date from 0001-01-01 through
// 9999-12-31. It never normalizes: February 30 is refused, not moved to March.
func NewDate(year int, month time.Month, day int) (Date, error) {
	d := Date{Year: year, Month: month, Day: day}
	if !d.valid() {
		return Date{}, ErrInvalidDate
	}

	return d, nil
}

// ParseDate parses s, which must be exactly YYYY-MM-DD: a real calendar date
// from 0001-01-01 through 9999-12-31, with no surrounding space, sign, time, or
// zone. Anything else returns the zero Date and ErrInvalidDate, whose text
// never includes s.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil || t.Format(time.DateOnly) != s {
		return Date{}, ErrInvalidDate
	}

	return NewDate(t.Date())
}

// DateOf returns the calendar day of t in UTC. A time outside 0001-01-01
// through 9999-12-31 gives a Date that Value and MarshalText refuse. Today's
// date is DateOf(ClockFrom(ctx).Now(ctx)).
func DateOf(t time.Time) Date {
	y, m, d := t.UTC().Date()

	return Date{Year: y, Month: m, Day: d}
}

// String returns d as YYYY-MM-DD, its fields as they are; the zero Date prints
// 0000-00-00.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// IsZero reports whether d is the zero Date. encoding/json's omitzero option
// calls it.
func (d Date) IsZero() bool {
	return d == Date{}
}

// Before reports whether d is an earlier calendar day than other.
func (d Date) Before(other Date) bool {
	return d.compare(other) < 0
}

// After reports whether d is a later calendar day than other.
func (d Date) After(other Date) bool {
	return d.compare(other) > 0
}

// Value implements driver.Valuer. It writes d as YYYY-MM-DD text, which every
// engine reads as the same day whatever the session time zone, and returns
// ErrInvalidDate for a Date that is not a real calendar date in range, the zero
// Date included.
func (d Date) Value() (driver.Value, error) {
	if !d.valid() {
		return nil, ErrInvalidDate
	}

	return d.String(), nil
}

// Scan implements sql.Scanner. It reads a time.Time as that time's own year,
// month, and day, never converted to another zone first (drivers return a date
// column as midnight UTC). It reads a string or []byte with ParseDate, using
// only its first ten bytes when it is longer, because a driver can return
// timestamp text for a date column. Anything else, nil included, and any date
// out of range, returns ErrInvalidDate and leaves d unchanged. A NULL belongs
// in a *Date, which database/sql and GORM set to nil without calling Scan.
func (d *Date) Scan(src any) error {
	var (
		parsed Date
		err    error
	)

	switch v := src.(type) {
	case time.Time:
		parsed, err = NewDate(v.Date())
	case string:
		parsed, err = parseDateColumn(v)
	case []byte:
		parsed, err = parseDateColumn(string(v))
	default:
		return ErrInvalidDate
	}

	if err != nil {
		return err
	}

	*d = parsed

	return nil
}

// GormDataType returns "date", so GORM declares a date column for a Date field
// (and a nullable one for a *Date field).
func (Date) GormDataType() string {
	return "date"
}

// MarshalText implements encoding.TextMarshaler, so JSON writes a Date as the
// string "YYYY-MM-DD". Like Value, it returns ErrInvalidDate for a Date that is
// not a real calendar date in range, the zero Date included.
func (d Date) MarshalText() ([]byte, error) {
	if !d.valid() {
		return nil, ErrInvalidDate
	}

	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with ParseDate. On an error
// it returns ErrInvalidDate and leaves d unchanged.
func (d *Date) UnmarshalText(text []byte) error {
	parsed, err := ParseDate(string(text))
	if err != nil {
		return err
	}

	*d = parsed

	return nil
}

// valid reports whether d is a real calendar date from 0001-01-01 through
// 9999-12-31.
func (d Date) valid() bool {
	if d.Year < 1 || d.Year > 9999 {
		return false
	}

	y, m, day := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Date()

	return y == d.Year && m == d.Month && day == d.Day
}

// compare orders d and other by year, then month, then day.
func (d Date) compare(other Date) int {
	if c := cmp.Compare(d.Year, other.Year); c != 0 {
		return c
	}

	if c := cmp.Compare(d.Month, other.Month); c != 0 {
		return c
	}

	return cmp.Compare(d.Day, other.Day)
}

// parseDateColumn parses s, or only its first ten bytes when it is longer.
func parseDateColumn(s string) (Date, error) {
	if len(s) > dateTextLength {
		s = s[:dateTextLength]
	}

	return ParseDate(s)
}
