// Package recurrence computes next occurrences of weekly recurring event
// patterns expressed as JSON. A pattern names a day of the week and a daily
// start time; occurrence math is performed in a caller-supplied time zone so
// results stay correct across daylight-saving-time transitions.
package recurrence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Sentinel errors for recurrence operations.
var (
	ErrEmptyPattern      = errors.New("recurrence: empty pattern")
	ErrUnknownDay        = errors.New("recurrence: unknown day_of_week")
	ErrTimeOutOfRange    = errors.New("recurrence: time out of range")
	ErrInvalidTimeFormat = errors.New("recurrence: expected HH:MM format")
)

// Pattern describes a weekly recurring event expressed as JSON.
// Example: {"day_of_week":"friday","start_time":"17:00","end_time":"22:00"}
type Pattern struct {
	DayOfWeek string `json:"day_of_week"` // "monday", "tuesday", ..., "sunday"
	StartTime string `json:"start_time"`  // "HH:MM" 24-hour
	EndTime   string `json:"end_time"`    // "HH:MM" 24-hour
}

// dayOfWeekMap maps lowercase day names to time.Weekday.
var dayOfWeekMap = map[string]time.Weekday{ //nolint:gochecknoglobals // intentional package-level lookup
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

// ParsePattern parses a JSON recurrence pattern.
func ParsePattern(raw json.RawMessage) (*Pattern, error) {
	if len(raw) == 0 {
		return nil, ErrEmptyPattern
	}
	var p Pattern
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("recurrence: failed to parse pattern: %w", err)
	}
	p.DayOfWeek = strings.ToLower(p.DayOfWeek)
	return &p, nil
}

// NextOccurrence computes the next occurrence of a recurring event from `after`,
// in the given timezone. Returns the date/time of the next occurrence with
// the pattern's start_time applied.
//
// Rules:
//   - If `after` is on the target day_of_week AND before start_time, returns today at start_time.
//   - Otherwise, returns the next future instance of the day_of_week at start_time.
//
// Always computes in the provided timezone to handle DST correctly.
func NextOccurrence(raw json.RawMessage, after time.Time, tz *time.Location) (time.Time, error) {
	if tz == nil {
		tz = time.UTC
	}

	p, err := ParsePattern(raw)
	if err != nil {
		return time.Time{}, err
	}

	targetWeekday, ok := dayOfWeekMap[p.DayOfWeek]
	if !ok {
		return time.Time{}, fmt.Errorf("%w: %s", ErrUnknownDay, p.DayOfWeek)
	}

	startHour, startMin, err := parseTimeHHMM(p.StartTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("recurrence: invalid start_time %q: %w", p.StartTime, err)
	}

	// Work in the target timezone
	now := after.In(tz)

	// Days until the next target weekday (0 = today)
	daysUntil := int(targetWeekday) - int(now.Weekday())
	if daysUntil < 0 {
		daysUntil += 7
	}

	// Candidate: target day at start_time in the requested timezone
	candidate := time.Date(
		now.Year(), now.Month(), now.Day()+daysUntil,
		startHour, startMin, 0, 0, tz,
	)

	// If the candidate is today (daysUntil == 0) and start_time has already passed,
	// advance by 7 days to next week.
	if daysUntil == 0 && !now.Before(candidate) {
		candidate = candidate.AddDate(0, 0, 7)
	}

	return candidate, nil
}

// parseTimeHHMM parses a "HH:MM" time string and returns hour and minute.
//
// It is a hand-rolled, allocation-free equivalent of
// fmt.Sscanf(s, "%d:%d", &h, &m): each half is an optional sign followed by one
// or more base-10 ASCII digits, the two halves are separated by a single ':',
// leading whitespace before each number is skipped, and trailing content after
// the minute is tolerated. Preserving those exact acceptance semantics keeps the
// change behavior-preserving while dropping the reflection and allocations of the
// fmt scanner on this hot path.
func parseTimeHHMM(s string) (int, int, error) {
	h, rest, ok := scanSignedInt(s)
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrInvalidTimeFormat, s)
	}
	if len(rest) == 0 || rest[0] != ':' {
		return 0, 0, fmt.Errorf("%w: %s", ErrInvalidTimeFormat, s)
	}
	m, _, ok := scanSignedInt(rest[1:])
	if !ok {
		return 0, 0, fmt.Errorf("%w: %s", ErrInvalidTimeFormat, s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("%w: %s", ErrTimeOutOfRange, s)
	}
	return h, m, nil
}

// scanSignedInt mirrors fmt's %d verb: it skips leading whitespace, accepts an
// optional single sign, then requires one or more base-10 ASCII digits. It
// returns the parsed value, the unconsumed remainder of s, and whether a valid
// integer (that also fits in an int) was scanned.
func scanSignedInt(s string) (int, string, bool) {
	s, ok := skipFmtSpace(s)
	if !ok {
		return 0, s, false
	}
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, s, false // no digits after the optional sign
	}
	v, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, s, false // overflow
	}
	return v, s[i:], true
}

// skipFmtSpace mirrors fmt's (*ss).SkipSpace for the Sscanf family, where a
// newline is not whitespace: it consumes leading whitespace runes and returns
// the remainder, or false if an unexpected newline (or a "\r\n") is reached.
func skipFmtSpace(s string) (string, bool) {
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == '\r' && strings.HasPrefix(s[size:], "\n"):
			s = s[size:] // consume the '\r'; the next iteration hits the '\n'
		case r == '\n':
			return s, false // Sscanf treats a bare newline as unexpected
		case !isFmtSpace(r):
			return s, true
		default:
			s = s[size:]
		}
	}
	return s, true
}

// fmtSpaceRanges mirrors fmt's private `space` table so skipFmtSpace skips
// exactly the runes fmt's scanner treats as space, which is a specific subset of
// unicode.IsSpace.
var fmtSpaceRanges = [...][2]rune{ //nolint:gochecknoglobals // mirrors fmt's private space table
	{0x0009, 0x000d},
	{0x0020, 0x0020},
	{0x0085, 0x0085},
	{0x00a0, 0x00a0},
	{0x1680, 0x1680},
	{0x2000, 0x200a},
	{0x2028, 0x2029},
	{0x202f, 0x202f},
	{0x205f, 0x205f},
	{0x3000, 0x3000},
}

// isFmtSpace reports whether r is whitespace by fmt's scanner rules.
func isFmtSpace(r rune) bool {
	if r >= 1<<16 {
		return false
	}
	for _, rng := range fmtSpaceRanges {
		if r < rng[0] {
			return false
		}
		if r <= rng[1] {
			return true
		}
	}
	return false
}
