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
		input string
		want  string
	}{
		{"2020-05-01T12:34:56.789+09:00", "2020-05-01T12:34:56.789+09:00"},
		{"2020-05-01T12:34:56+09:00", "2020-05-01T12:34:56.000+09:00"},
		{"2020-05-01T12:34:56.789Z", "2020-05-01T21:34:56.789+09:00"},
		{"2020-05-01", "2020-05-01T00:00:00.000+09:00"},
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
		})
	}
	for _, input := range []string{"2020/05/01", "2020-05-01T12:34:56.78Z", "2020-05-01T12:34:56.7891+09:00", "2020-05-01T12:34:56.789+09:00junk"} {
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
