// Package kftime provides kickflow-compatible time formatting and parsing.
package kftime

import (
	"fmt"
	"time"
)

const (
	timestampLayout = "2006-01-02T15:04:05.000-07:00"
	dateLayout      = "2006-01-02"
)

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the system wall clock.
type SystemClock struct{}

// Now returns the current system time.
func (SystemClock) Now() time.Time { return time.Now() }

// JST is a fixed UTC+09:00 location, independent of the host timezone database.
var JST = time.FixedZone("JST", 9*60*60)

// Format formats an instant in JST, truncating sub-millisecond precision.
func Format(t time.Time) string {
	return t.In(JST).Truncate(time.Millisecond).Format(timestampLayout)
}

// FormatDate formats a date in JST.
func FormatDate(t time.Time) string { return t.In(JST).Format(dateLayout) }

// ParseInput parses one of the timestamp formats accepted by kickflow.
func ParseInput(s string) (time.Time, error) {
	var layout string
	switch len(s) {
	case 29:
		layout = timestampLayout
	case 25:
		layout = "2006-01-02T15:04:05-07:00"
	case 24:
		layout = "2006-01-02T15:04:05.000Z"
	case 10:
		layout = dateLayout
	default:
		return time.Time{}, fmt.Errorf("unsupported time format %q", s)
	}
	parsed, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("unsupported time format %q: %w", s, err)
	}
	if layout == dateLayout {
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, JST), nil
	}
	return parsed, nil
}

// ToMillis converts an instant to Unix milliseconds.
func ToMillis(t time.Time) int64 { return t.UnixMilli() }

// FromMillis converts Unix milliseconds to an instant.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms) }
