package kftime

import (
	"testing"
	"time"
)

func TestFormatUsesJSTAndTruncatesToMilliseconds(t *testing.T) {
	timeValue := time.Date(2020, 5, 1, 3, 34, 56, 789123000, time.UTC)
	if got, want := Format(timeValue), "2020-05-01T12:34:56.789+09:00"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
	if got, want := Format(time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)), "2020-05-01T09:00:00.000+09:00"; got != want {
		t.Fatalf("Format() with zero nanos = %q, want %q", got, want)
	}
}

func TestParseInputAcceptedFormatsAndRejectsOthers(t *testing.T) {
	cases := []struct {
		input      string
		want       string
		nanosecond int
	}{
		{"2020-05-01T12:34:56.789+09:00", "2020-05-01T12:34:56.789+09:00", 789000000},
		{"2020-05-01T12:34:56+09:00", "2020-05-01T12:34:56.000+09:00", 0},
		{"2020-05-01T12:34:56.789Z", "2020-05-01T21:34:56.789+09:00", 789000000},
		{"2020-05-01", "2020-05-01T00:00:00.000+09:00", 0},
		{"2020-05-01T12:34:56Z", "2020-05-01T21:34:56.000+09:00", 0},
		{"2020-05-01T12:34:56.7Z", "2020-05-01T21:34:56.700+09:00", 700000000},
		{"2020-05-01T12:34:56.78Z", "2020-05-01T21:34:56.780+09:00", 780000000},
		{"2020-05-01T12:34:56.7891+09:00", "2020-05-01T12:34:56.789+09:00", 789100000},
		{"2026-10-28T04:08:57.101584+00:00", "2026-10-28T13:08:57.101+09:00", 101584000},
		{"2020-05-01T12:34:56.123456789-05:30", "2020-05-02T03:04:56.123+09:00", 123456789},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseInput(tc.input)
			if err != nil {
				t.Fatalf("ParseInput() error = %v", err)
			}
			if formatted := Format(got); formatted != tc.want {
				t.Fatalf("Format(ParseInput()) = %q, want %q", formatted, tc.want)
			}
			if got.Nanosecond() != tc.nanosecond {
				t.Errorf("Nanosecond() = %d, want %d", got.Nanosecond(), tc.nanosecond)
			}
		})
	}
	for _, input := range []string{
		"2020/05/01", "2020-05-01T12:34:56.789+09:00junk", "2020-05-01t12:34:56Z",
		"2020-05-01T12:34:56z", "2020-05-01 12:34:56Z", "2020-05-01T12:34:56",
		"2020-05-01T12:34:56,789Z", "2020-05-01T12:34:56.1234567890Z",
		"2020-05-01T12:34:56.Z", "2020-05-01T12:34:56+0900", "2020-13-01T00:00:00Z",
		"2020-05-01T24:00:00Z", "", "garbage",
	} {
		if _, err := ParseInput(input); err == nil {
			t.Errorf("ParseInput(%q) accepted unsupported format", input)
		}
	}
}

func TestFormatDateAndMillis(t *testing.T) {
	value := time.Date(2020, 5, 1, 20, 0, 0, 0, time.UTC)
	if got, want := FormatDate(value), "2020-05-02"; got != want {
		t.Fatalf("FormatDate() = %q, want %q", got, want)
	}
	if got := FromMillis(ToMillis(value)); !got.Equal(value) {
		t.Fatalf("FromMillis(ToMillis()) = %v, want %v", got, value)
	}
}

func TestFakeClock(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	clock.Advance(time.Hour)
	if want := start.Add(time.Hour); !clock.Now().Equal(want) {
		t.Fatalf("Now() = %v, want %v", clock.Now(), want)
	}
	clock.Set(start)
	if !clock.Now().Equal(start) {
		t.Fatalf("Set() did not update clock: got %v, want %v", clock.Now(), start)
	}
}
