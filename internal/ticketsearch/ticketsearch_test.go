package ticketsearch

import (
	"reflect"
	"strings"
	"testing"
)

func TestTerms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{}},
		{"blank", "  \t ", []string{}},
		{"single", "offsite", []string{"offsite"}},
		{"space separated", "a  b\tc", []string{"a", "b", "c"}},
		{"full-width space", "予算　会食", []string{"予算", "会食"}},
		{"leading and trailing", "  a b ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Terms(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Terms(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}

	t.Run("term count is bounded", func(t *testing.T) {
		got := Terms(strings.Repeat("x ", MaxTerms+5))
		if len(got) != MaxTerms {
			t.Fatalf("len = %d, want %d", len(got), MaxTerms)
		}
	})
	t.Run("term length is bounded in runes", func(t *testing.T) {
		got := Terms(strings.Repeat("あ", MaxTermRunes+10))
		if len([]rune(got[0])) != MaxTermRunes {
			t.Fatalf("rune length = %d, want %d", len([]rune(got[0])), MaxTermRunes)
		}
	})
}

func TestValueText(t *testing.T) {
	if got := valueText(nil); got != "" {
		t.Fatalf("nil -> %q", got)
	}
	if got := valueText("abc"); got != "abc" {
		t.Fatalf("string -> %q", got)
	}
	if got := valueText(12.5); got != "12.5" {
		t.Fatalf("number -> %q", got)
	}
	if got := valueText([]any{"a", "b"}); got != `["a","b"]` {
		t.Fatalf("array -> %q", got)
	}
}
