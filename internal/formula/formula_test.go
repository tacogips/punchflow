package formula

import (
	"errors"
	"testing"
)

type mapEnv map[string]any

func (m mapEnv) Lookup(name string) (any, bool) { v, ok := m[name]; return v, ok }

func TestEval(t *testing.T) {
	env := mapEnv{
		"qty": "3", "price": 1000.5, "empty": "", "text": "abc", "expense-date": "2025-02-03",
		"lines.qty": []any{"2", 3.0, nil}, "lines.price": []any{"10.5", "4"},
	}
	cases := []struct {
		src  string
		want any
	}{
		{"qty * price", 3001.5},
		{"(1 + 2) * 3 - 4 / 2", 7.0},
		{"-qty + 10", 7.0},
		{"0.1 + 0.2", 0.3},
		{"empty + 5", 5.0},
		{"SUM(lines.qty)", 5.0},
		{"sum(lines.price) * 2", 29.0},
		{"AVG(lines.qty)", 2.5},
		{"MAX(lines.qty, 10)", 10.0},
		{"MIN(lines.qty)", 2.0},
		{"COUNT(lines.qty)", 2.0},
		{"ROUND(2.5)", 3.0},
		{"ROUND(1.005, 2)", 1.01},
		{"ROUNDUP(1.21, 1)", 1.3},
		{"ROUNDDOWN(-1.29, 1)", -1.2},
		{"ROUNDUP(-1.21, 1)", -1.3},
		{"ROUND(1234, -2)", 1200.0},
		{"FLOOR(2.9) + CEIL(2.1) + ABS(-1)", 6.0},
		{"IF(qty > 2, \"many\", \"few\")", "many"},
		{"IF(qty > 5, 1/0, 2)", 2.0},
		{"IF(AND(qty = 3, NOT(FALSE)), 1, 0)", 1.0},
		{"qty >= 3 && price < 1000 || TRUE", true},
		{"text & \"-\" & qty", "abc-3"},
		{"CONCAT(text, 1, {expense-date})", "abc12025-02-03"},
		{"[expense-date] = \"2025-02-03\"", true},
		{"5 % 3", 2.0},
		{"'it\\'s'", "it's"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			expr, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, err := expr.Eval(env)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v (%T), want %v", got, got, tc.want)
			}
		})
	}
}

func TestParseUnsupported(t *testing.T) {
	for _, src := range []string{"", "1 +", "foo(1)", "json_lookup(a, b)", "ROUND()", "(1", "1 2", "a @ b", "\"open", "{}"} {
		_, err := Parse(src)
		var u *UnsupportedError
		if !errors.As(err, &u) {
			t.Errorf("Parse(%q) error = %v, want UnsupportedError", src, err)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	env := mapEnv{"text": "abc", "list": []any{1.0, 2.0}}
	for _, src := range []string{"1 / 0", "text * 2", "list + 1", "missing + 1", "SUM(text)"} {
		expr, err := Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		var e *EvalError
		if _, err = expr.Eval(env); !errors.As(err, &e) {
			t.Errorf("Eval(%q) error = %v, want EvalError", src, err)
		}
	}
}

func TestRefs(t *testing.T) {
	expr, err := Parse("SUM(lines.qty) * {unit-price} + qty + qty")
	if err != nil {
		t.Fatal(err)
	}
	got := expr.Refs()
	want := []string{"lines.qty", "unit-price", "qty"}
	if len(got) != len(want) {
		t.Fatalf("refs=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refs=%v, want %v", got, want)
		}
	}
}
