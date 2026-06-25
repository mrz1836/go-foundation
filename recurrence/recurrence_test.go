package recurrence_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mrz1836/go-foundation/recurrence"
)

// easternTZ loads the America/New_York location for DST-sensitive assertions.
func easternTZ(t *testing.T) *time.Location {
	t.Helper()

	tz, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	return tz
}

// makePattern builds a JSON recurrence pattern as json.RawMessage.
func makePattern(day, start, end string) json.RawMessage {
	return json.RawMessage([]byte(`{"day_of_week":"` + day + `","start_time":"` + start + `","end_time":"` + end + `"}`))
}

func TestNextOccurrence_NextFridayFromMonday(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)

	// Monday 2025-03-10 09:00 Eastern
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, tz)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect Friday 2025-03-14 17:00 Eastern
	expected := time.Date(2025, 3, 14, 17, 0, 0, 0, tz)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrence_FridayBeforeStartTime(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)

	// Friday 2025-03-14 12:00 Eastern (before 17:00 start)
	after := time.Date(2025, 3, 14, 12, 0, 0, 0, tz)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return same Friday at 17:00
	expected := time.Date(2025, 3, 14, 17, 0, 0, 0, tz)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrence_FridayAfterStartTime(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)

	// Friday 2025-03-14 19:00 Eastern (after 17:00 start)
	after := time.Date(2025, 3, 14, 19, 0, 0, 0, tz)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return next Friday (March 21)
	expected := time.Date(2025, 3, 21, 17, 0, 0, 0, tz)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrence_FridayExactlyAtStartTime(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)

	// Friday 2025-03-14 17:00:00 Eastern — exactly at start, should advance
	after := time.Date(2025, 3, 14, 17, 0, 0, 0, tz)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At exactly start_time, "before" is false → advance to next Friday
	expected := time.Date(2025, 3, 21, 17, 0, 0, 0, tz)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrence_DSTSpringForward(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)

	// Saturday 2025-03-08 09:00 Eastern (day before DST spring-forward on March 9, 2025)
	after := time.Date(2025, 3, 8, 9, 0, 0, 0, tz)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next Friday is March 14, 2025 — after spring forward, should be EDT (UTC-4)
	expected := time.Date(2025, 3, 14, 17, 0, 0, 0, tz)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
	// The hour in UTC should be 21:00 (EDT is UTC-4)
	if next.UTC().Hour() != 21 {
		t.Errorf("expected UTC hour 21 (EDT), got %d", next.UTC().Hour())
	}
}

func TestNextOccurrence_EmptyPattern(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, tz)

	_, err := recurrence.NextOccurrence(json.RawMessage(nil), after, tz)
	if err == nil {
		t.Error("expected error for empty pattern")
	}
	if !errors.Is(err, recurrence.ErrEmptyPattern) {
		t.Errorf("expected ErrEmptyPattern, got %v", err)
	}
}

func TestNextOccurrence_UnknownDay(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, tz)
	pattern := makePattern("funday", "17:00", "22:00")

	_, err := recurrence.NextOccurrence(pattern, after, tz)
	if err == nil {
		t.Error("expected error for unknown day_of_week")
	}
	if !errors.Is(err, recurrence.ErrUnknownDay) {
		t.Errorf("expected ErrUnknownDay, got %v", err)
	}
}

// TestNextOccurrence_NilTimezoneDefaultsToUTC verifies that a nil *time.Location
// is treated as UTC.
func TestNextOccurrence_NilTimezoneDefaultsToUTC(t *testing.T) {
	t.Parallel()

	// Monday 2025-03-10 09:00 UTC
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC)
	pattern := makePattern("friday", "17:00", "22:00")

	next, err := recurrence.NextOccurrence(pattern, after, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should be computed in UTC.
	if next.Location() != time.UTC {
		t.Errorf("expected UTC location, got %v", next.Location())
	}

	expected := time.Date(2025, 3, 14, 17, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

// TestNextOccurrence_InvalidTimeFormat verifies that a non-"HH:MM" start_time
// surfaces ErrInvalidTimeFormat.
func TestNextOccurrence_InvalidTimeFormat(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, tz)
	pattern := makePattern("friday", "abc", "22:00")

	_, err := recurrence.NextOccurrence(pattern, after, tz)
	if err == nil {
		t.Fatal("expected error for invalid start_time format")
	}
	if !errors.Is(err, recurrence.ErrInvalidTimeFormat) {
		t.Errorf("expected ErrInvalidTimeFormat, got %v", err)
	}
}

// TestNextOccurrence_TimeOutOfRange verifies that an out-of-range start_time
// surfaces ErrTimeOutOfRange.
func TestNextOccurrence_TimeOutOfRange(t *testing.T) {
	t.Parallel()

	tz := easternTZ(t)
	after := time.Date(2025, 3, 10, 9, 0, 0, 0, tz)
	pattern := makePattern("friday", "25:61", "22:00")

	_, err := recurrence.NextOccurrence(pattern, after, tz)
	if err == nil {
		t.Fatal("expected error for out-of-range start_time")
	}
	if !errors.Is(err, recurrence.ErrTimeOutOfRange) {
		t.Errorf("expected ErrTimeOutOfRange, got %v", err)
	}
}

func TestParsePattern_Success(t *testing.T) {
	t.Parallel()

	// Day name with mixed case to exercise the lowercasing path.
	p, err := recurrence.ParsePattern(makePattern("Friday", "17:00", "21:30"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DayOfWeek != "friday" {
		t.Errorf("expected normalized day 'friday', got %q", p.DayOfWeek)
	}
	if p.StartTime != "17:00" || p.EndTime != "21:30" {
		t.Errorf("unexpected pattern fields: %+v", p)
	}
}

func TestParsePattern_Empty(t *testing.T) {
	t.Parallel()

	_, err := recurrence.ParsePattern(json.RawMessage(nil))
	if !errors.Is(err, recurrence.ErrEmptyPattern) {
		t.Errorf("expected ErrEmptyPattern, got %v", err)
	}
}

func TestParsePattern_MalformedJSON(t *testing.T) {
	t.Parallel()

	_, err := recurrence.ParsePattern(json.RawMessage([]byte("{not-json")))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	// Malformed JSON must not masquerade as one of the validation sentinels.
	if errors.Is(err, recurrence.ErrEmptyPattern) {
		t.Errorf("malformed JSON should not report ErrEmptyPattern, got %v", err)
	}
}
