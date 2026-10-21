package export

import (
	"bytes"
	"strings"
	"testing"
)

func TestSanitizeCell(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"=SUM(A1)", "'=SUM(A1)"},
		{"+81 3 1234", "'+81 3 1234"},
		{"@user", "'@user"},
		{"\tcell", "'\tcell"},
		{"\rcell", "'\rcell"},
		{"-5", "-5"},
		{"-1,234.5", "-1,234.5"},
		{"-cmd", "'-cmd"},
		{"- 1", "'- 1"},
		{"a=b", "a=b"},
		{"日本語", "日本語"},
	}
	for _, tt := range tests {
		if got := SanitizeCell(tt.in); got != tt.want {
			t.Errorf("SanitizeCell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriterFraming(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Write([]string{"a", "b,c", `d"e`, "line1\nline2"}); err != nil {
		t.Fatal(err)
	}
	if err = w.Write([]string{"=x", "日本"}); err != nil {
		t.Fatal(err)
	}
	if err = w.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\xEF\xBB\xBF") {
		t.Fatalf("missing BOM: %q", got)
	}
	want := "\xEF\xBB\xBFa,\"b,c\",\"d\"\"e\",\"line1\r\nline2\"\r\n'=x,日本\r\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
