package formula

import (
	"errors"
	"reflect"
	"testing"
	"time"
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
	for _, src := range []string{"", "1 +", "foo(1)", "ROUND()", "(1", "1 2", "a @ b", "\"open", "{}"} {
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

// TestKickflowManualExamples keeps the expressions in help-center article
// 360053280013 executable. Formula strings and expected results follow the
// manual verbatim (comments/results omitted from each input string).
func TestKickflowManualExamples(t *testing.T) {
	env := mapEnv{
		"num": 100.0, "text": "", "x": "", "emptyText": "", "emptyNull": nil,
		"author":     `{"id":"12345678-abcd-efgh-1234-123456789012","firstName":"太郎","lastName":"山田","fullName":"山田 太郎","email":"taro.yamada@example.com","code":"xxxxxxxxxxxxxxxx","employeeId":"12345678","customFields":{"kana":"ヤマダ タロウ","hire_type":"正社員"}}`,
		"authorTeam": `{"id":"12345678-abcd-efgh-1234-123456789012","name":"◯◯部","fullName":"株式会社とくがわ / ◯◯部","code":"xxxxxxxxxxxxxxxx"}`,
		"jsonStr": `[
    {"name":"Taro", "age": 20},
    {"name":"Jiro", "age": 18},
    {"name":"Hanako", "age": 22}
  ]`,
	}
	cases := []struct {
		src  string
		want any
	}{
		{`round(3.14)`, 3.0}, {`round(3.144, 2)`, 3.14}, {`round(3.145, 2)`, 3.15}, {`round(123, -1)`, 120.0},
		{`ceil(3.14)`, 4.0}, {`ceil(3.144, 2)`, 3.15}, {`ceil(3.145, 2)`, 3.15}, {`ceil(123, -1)`, 130.0},
		{`floor(3.14)`, 3.0}, {`floor(3.144, 2)`, 3.14}, {`floor(3.145, 2)`, 3.14}, {`floor(123, -1)`, 120.0},
		{`max(1, 2, 3)`, 3.0}, {`min(1, 2, 3)`, 1.0}, {`mod(10, 3)`, 1.0},
		{`if($num >= 100, 1, 0)`, 1.0}, {`if(exact($text, ""), "Empty", "Not empty")`, "Empty"},
		{`and(1 == 1)`, true}, {`and(1 == 1, 2 == 2)`, true}, {`and(1 == 1, 2 == 3)`, false}, {`and(0)`, false},
		{`or(1 == 1)`, true}, {`or(1 == 1, 2 != 2)`, true}, {`or(1 == 2, 2 == 3)`, false}, {`or(0)`, false},
		{`not(1 == 2)`, true}, {`not(1 == 1)`, false}, {`not(0)`, true}, {`not("")`, true},
		{`exact("hoge", "hoge")`, true}, {`exact("hoge", "fuga")`, false}, {`exact(x, "")`, true}, {`exact($emptyNull, "")`, true},
		{`concat("a", "b", "c")`, "abc"}, {`concat("abc", 123)`, "abc123"},
		{`left("あいうえお", 3)`, "あいう"}, {`right("あいうえお", 3)`, "うえお"}, {`mid("あいうえお", 2, 3)`, "いうえ"},
		{`text(1.2345, "0.##")`, "1.23"}, {`text(1234567, "#,###円")`, "1,234,567円"}, {`text(123, "00000")`, "00123"}, {`text("2024-01-01", "yyyy/mm/dd")`, "2024/01/01"}, {`text("2022-04-08T13:04:56", "hh:mm AM/PM")`, "01:04 午後"},
		{`json_lookup($author, "$.fullName")`, "山田 太郎"}, {`json_lookup($author, "$.customFields.kana")`, "ヤマダ タロウ"},
		{`json_lookup($authorTeam, "$.fullName")`, "株式会社とくがわ / ◯◯部"}, {`json_lookup($authorTeam, "$.name")`, "◯◯部"},
		{`json_lookup($jsonStr, "$[0].name")`, "Taro"}, {`json_lookup($jsonStr, "$[1].age")`, float64(18)}, {`json_lookup($jsonStr, "$[2]")`, `{"age":22,"name":"Hanako"}`},
		{`year("2022-01-02")`, 2022.0}, {`month("2022-01-02")`, 1.0}, {`day("2022-01-02")`, 2.0}, {`weekday("2022-01-02")`, 1.0},
		{`datedif("2022-01-01", "2022-01-31", "D")`, 30.0}, {`datedif("2022-01-01", "2022-04-01", "M")`, 3.0}, {`datedif("2022-01-01", "2023-01-01", "Y")`, 1.0},
		{`dateadd("2024-01-01", 1)`, "2024-01-02"}, {`dateadd("2024-01-01", -1)`, "2023-12-31"}, {`edate("2024-01-01", 1)`, "2024-02-01"}, {`edate("2024-01-01", -1)`, "2023-12-01"},
		{`datetimedif("2024-03-22 09:00", "2024-03-22 17:30")`, 510.0}, {`datetimedif("2024-03-22 22:00", "2024-03-23 06:00")`, 480.0},
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
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
	got, err := Parse(`today()`)
	if err != nil {
		t.Fatal(err)
	}
	value, err := got.Eval(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse("2006-01-02", value.(string)); err != nil {
		t.Fatalf("today()=%v, want ISO date", value)
	}
}

func TestKickflowDollarReferencesAndDetailSums(t *testing.T) {
	env := mapEnv{"code": "12", "units": []any{10.0, 2.5}, "unit-price": []any{10.0, 2.5}}
	for _, tc := range []struct {
		src  string
		want any
	}{{`$code + 1`, 13.0}, {`${code} + 1`, 13.0}, {`$sum$units`, 12.5}, {`$sum${unit-price}`, 12.5}} {
		expr, err := Parse(tc.src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.src, err)
		}
		got, err := expr.Eval(env)
		if err != nil {
			t.Fatalf("Eval(%q): %v", tc.src, err)
		}
		if got != tc.want {
			t.Errorf("Eval(%q)=%v want %v", tc.src, got, tc.want)
		}
	}
}
