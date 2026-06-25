// Package recurrence computes next occurrences of weekly recurring event
// patterns expressed as JSON. A pattern names a day of the week and a daily
// start time; occurrence math is performed in a caller-supplied time zone so
// results stay correct across daylight-saving-time transitions.
package recurrence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
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
func parseTimeHHMM(s string) (int, int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, 0, fmt.Errorf("%w: %s: %w", ErrInvalidTimeFormat, s, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("%w: %s", ErrTimeOutOfRange, s)
	}
	return h, m, nil
}
