// Package kftime provides kickflow-compatible time formatting and parsing.
package kftime

import (
	"fmt"
	"regexp"
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

var rfc3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$`)

// ParseInput parses an RFC 3339 date-time or a date at midnight JST.
func ParseInput(s string) (time.Time, error) {
	if len(s) == 10 {
		parsed, err := time.Parse(dateLayout, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("unsupported time format %q: %w", s, err)
		}
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, JST), nil
	}
	if !rfc3339Pattern.MatchString(s) {
		return time.Time{}, fmt.Errorf("unsupported time format %q", s)
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("unsupported time format %q: %w", s, err)
	}
	return parsed, nil
}

// ToMillis converts an instant to Unix milliseconds.
func ToMillis(t time.Time) int64 { return t.UnixMilli() }

// FromMillis converts Unix milliseconds to an instant.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms) }
