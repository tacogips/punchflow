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

func TestAuthorTeamNameFallsBackToSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		detail map[string]any
		want   string
	}{
		{name: "live team", detail: map[string]any{"authorTeam": map[string]any{"fullName": "Sales"}, "authorTeamSnapshot": map[string]any{"fullName": "Former Sales"}}, want: "Sales"},
		{name: "deleted team", detail: map[string]any{"authorTeam": nil, "authorTeamSnapshot": map[string]any{"fullName": "Former Sales"}}, want: "Former Sales"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixedCells(tt.detail, LayoutTicket)
			if got[7] != tt.want {
				t.Fatalf("CSV author-team cell = %q, want %q", got[7], tt.want)
			}
		})
	}
}
