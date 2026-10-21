package formula

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Env resolves formula references. Lookup returns a scalar (nil, float64,
// string or bool) or a []any column, and false for unknown names.
type Env interface {
	Lookup(name string) (any, bool)
}

// EvalError reports a runtime failure such as division by zero or a
// non-numeric operand. Callers store null for such results.
type EvalError struct{ Reason string }

func (e *EvalError) Error() string { return "formula evaluation failed: " + e.Reason }

func evalFail(format string, args ...any) error {
	return &EvalError{Reason: fmt.Sprintf(format, args...)}
}

type node interface{}

type (
	literalNode struct{ value any }
	refNode     struct{ name string }
	sumRefNode  struct{ name string }
	unaryNode   struct {
		op      string
		operand node
	}
	binaryNode struct {
		op          string
		left, right node
	}
	callNode struct {
		name string
		args []node
	}
)

type funcSpec struct{ minArgs, maxArgs int }

var functions = map[string]funcSpec{
	"SUM": {1, -1}, "AVG": {1, -1}, "AVERAGE": {1, -1}, "MIN": {1, -1}, "MAX": {1, -1}, "COUNT": {1, -1},
	"ROUND": {1, 2}, "ROUNDUP": {1, 2}, "ROUNDDOWN": {1, 2}, "FLOOR": {1, 2}, "CEIL": {1, 2}, "CEILING": {1, 2}, "ABS": {1, 1}, "MOD": {2, 2},
	"IF": {3, 3}, "AND": {1, -1}, "OR": {1, -1}, "NOT": {1, 1}, "EXACT": {2, 2}, "CONCAT": {1, -1},
	"LEFT": {2, 2}, "RIGHT": {2, 2}, "MID": {3, 3}, "TEXT": {2, 2}, "JSON_LOOKUP": {2, 2},
	"TODAY": {0, 0}, "YEAR": {1, 1}, "MONTH": {1, 1}, "DAY": {1, 1}, "WEEKDAY": {1, 1}, "DATEDIF": {3, 3}, "DATEADD": {2, 2}, "EDATE": {2, 2}, "DATETIMEDIF": {2, 2},
}

// Eval evaluates the expression. The result is a float64, string or bool.
// Empty operands count as 0 in arithmetic.
func (e *Expr) Eval(env Env) (any, error) {
	v, err := eval(e.root, env)
	if err != nil {
		return nil, err
	}
	if cols, ok := v.([]any); ok {
		if len(cols) == 1 {
			return cols[0], nil
		}
		return nil, evalFail("result is a list")
	}
	if f, ok := v.(float64); ok {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, evalFail("result is not finite")
		}
		return clean(f), nil
	}
	return v, nil
}

// FormatValue renders an evaluation result as text (used for titles).
func FormatValue(v any) string { return toString(v) }

func clean(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	y, err := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 14, 64), 64)
	if err != nil {
		return x
	}
	return y
}

func eval(n node, env Env) (any, error) {
	switch v := n.(type) {
	case literalNode:
		return v.value, nil
	case refNode:
		val, ok := env.Lookup(v.name)
		if !ok {
			return nil, evalFail("unknown reference %q", v.name)
		}
		return val, nil
	case sumRefNode:
		val, ok := env.Lookup(v.name)
		if !ok {
			return nil, evalFail("unknown reference %q", v.name)
		}
		return aggregate("SUM", []any{val})
	case unaryNode:
		return evalUnary(v, env)
	case binaryNode:
		return evalBinary(v, env)
	case callNode:
		return evalCall(v, env)
	}
	return nil, evalFail("invalid expression")
}

func evalUnary(n unaryNode, env Env) (any, error) {
	val, err := eval(n.operand, env)
	if err != nil {
		return nil, err
	}
	if n.op == "!" {
		return !truthy(val), nil
	}
	f, err := toNumber(val)
	if err != nil {
		return nil, err
	}
	if n.op == "-" {
		return -f, nil
	}
	return f, nil
}

func evalBinary(n binaryNode, env Env) (any, error) {
	if n.op == "&&" || n.op == "||" {
		l, err := eval(n.left, env)
		if err != nil {
			return nil, err
		}
		if n.op == "&&" && !truthy(l) {
			return false, nil
		}
		if n.op == "||" && truthy(l) {
			return true, nil
		}
		r, err := eval(n.right, env)
		if err != nil {
			return nil, err
		}
		return truthy(r), nil
	}
	l, err := eval(n.left, env)
	if err != nil {
		return nil, err
	}
	r, err := eval(n.right, env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "&":
		return toString(l) + toString(r), nil
	case "<", "<=", ">", ">=", "=", "==", "!=", "<>":
		return compare(n.op, l, r), nil
	}
	a, err := toNumber(l)
	if err != nil {
		return nil, err
	}
	b, err := toNumber(r)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/", "%":
		if b == 0 {
			return nil, evalFail("division by zero")
		}
		if n.op == "%" {
			return math.Mod(a, b), nil
		}
		return a / b, nil
	}
	return nil, evalFail("unknown operator %q", n.op)
}

func compare(op string, l, r any) bool {
	var cmp int
	_, lerr := toNumber(l)
	_, rerr := toNumber(r)
	if lerr == nil && rerr == nil {
		a, _ := toNumber(l)
		b, _ := toNumber(r)
		switch {
		case clean(a) < clean(b):
			cmp = -1
		case clean(a) > clean(b):
			cmp = 1
		}
	} else {
		cmp = strings.Compare(toString(l), toString(r))
	}
	switch op {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "=", "==":
		return cmp == 0
	}
	return cmp != 0
}

func evalCall(n callNode, env Env) (any, error) {
	if n.name == "IF" {
		c, err := eval(n.args[0], env)
		if err != nil {
			return nil, err
		}
		if truthy(c) {
			return eval(n.args[1], env)
		}
		return eval(n.args[2], env)
	}
	args := make([]any, len(n.args))
	for i, a := range n.args {
		v, err := eval(a, env)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	switch n.name {
	case "SUM", "AVG", "AVERAGE", "MIN", "MAX", "COUNT":
		return aggregate(n.name, args)
	case "AND", "OR":
		for _, a := range flatten(args) {
			if truthy(a) != (n.name == "AND") {
				return n.name == "OR", nil
			}
		}
		return n.name == "AND", nil
	case "NOT":
		return !truthy(args[0]), nil
	case "CONCAT":
		var sb strings.Builder
		for _, a := range flatten(args) {
			sb.WriteString(toString(a))
		}
		return sb.String(), nil
	case "MOD":
		x, err := toNumber(args[0])
		if err != nil {
			return nil, err
		}
		y, err := toNumber(args[1])
		if err != nil {
			return nil, err
		}
		if y == 0 {
			return nil, evalFail("division by zero")
		}
		return math.Mod(x, y), nil
	case "EXACT":
		if args[0] == nil && toString(args[1]) == "" {
			return true, nil
		}
		if args[1] == nil && toString(args[0]) == "" {
			return true, nil
		}
		if _, ok := args[0].(string); !ok {
			return nil, evalFail("exact expects strings")
		}
		if _, ok := args[1].(string); !ok {
			return nil, evalFail("exact expects strings")
		}
		return args[0] == args[1], nil
	case "LEFT", "RIGHT", "MID":
		return stringSlice(n.name, args)
	case "TEXT":
		return formatText(args[0], toString(args[1]))
	case "JSON_LOOKUP":
		return jsonLookup(args[0], toString(args[1]))
	case "TODAY":
		return time.Now().Format("2006-01-02"), nil
	case "YEAR", "MONTH", "DAY", "WEEKDAY":
		date, err := parseDate(args[0])
		if err != nil {
			return nil, err
		}
		switch n.name {
		case "YEAR":
			return float64(date.Year()), nil
		case "MONTH":
			return float64(date.Month()), nil
		case "DAY":
			return float64(date.Day()), nil
		default:
			return float64(date.Weekday() + 1), nil
		}
	case "DATEADD", "EDATE":
		date, err := parseDate(args[0])
		if err != nil {
			return nil, err
		}
		num, err := toNumber(args[1])
		if err != nil {
			return nil, err
		}
		if n.name == "DATEADD" {
			date = date.AddDate(0, 0, int(num))
		} else {
			day := date.Day()
			date = time.Date(date.Year(), date.Month()+time.Month(int(num)), 1, 0, 0, 0, 0, time.UTC)
			lastDay := time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
			if day > lastDay {
				day = lastDay
			}
			date = time.Date(date.Year(), date.Month(), day, 0, 0, 0, 0, time.UTC)
		}
		if date.Year() < 1900 || date.Year() > 9999 {
			return nil, evalFail("date is outside supported range")
		}
		return date.Format("2006-01-02"), nil
	case "DATEDIF":
		from, err := parseDate(args[0])
		if err != nil {
			return nil, err
		}
		to, err := parseDate(args[1])
		if err != nil {
			return nil, err
		}
		if to.Before(from) {
			return nil, evalFail("to is before from")
		}
		unit := strings.ToUpper(toString(args[2]))
		switch unit {
		case "D":
			return float64(int(to.Sub(from).Hours() / 24)), nil
		case "M":
			months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month())
			if to.Day() < from.Day() {
				months--
			}
			return float64(months), nil
		case "Y":
			years := to.Year() - from.Year()
			if to.Month() < from.Month() || to.Month() == from.Month() && to.Day() < from.Day() {
				years--
			}
			return float64(years), nil
		default:
			return nil, evalFail("unsupported datedif unit")
		}
	case "DATETIMEDIF":
		from, err := time.Parse("2006-01-02 15:04", toString(args[0]))
		if err != nil {
			return nil, evalFail("invalid datetime")
		}
		to, err := time.Parse("2006-01-02 15:04", toString(args[1]))
		if err != nil {
			return nil, evalFail("invalid datetime")
		}
		if to.Before(from) {
			return nil, evalFail("from is after to")
		}
		return float64(to.Sub(from).Minutes()), nil
	}
	x, err := toNumber(args[0])
	if err != nil {
		return nil, err
	}
	digits := 0.0
	if len(args) == 2 {
		if digits, err = toNumber(args[1]); err != nil {
			return nil, err
		}
	}
	p := math.Pow(10, math.Trunc(digits))
	scaled := clean(x * p)
	switch n.name {
	case "ROUND":
		return math.Round(scaled) / p, nil
	case "ROUNDUP":
		return math.Copysign(math.Ceil(math.Abs(scaled)), scaled) / p, nil
	case "ROUNDDOWN":
		return math.Trunc(scaled) / p, nil
	case "FLOOR":
		return roundDirected(x, digits, math.Floor), nil
	case "CEIL", "CEILING":
		return roundDirected(x, digits, math.Ceil), nil
	}
	return math.Abs(x), nil
}

func roundDirected(x, digits float64, fn func(float64) float64) float64 {
	p := math.Pow(10, math.Trunc(digits))
	return fn(x*p) / p
}
func parseDate(v any) (time.Time, error) {
	d, err := time.Parse("2006-01-02", toString(v))
	if err != nil {
		return time.Time{}, evalFail("invalid date")
	}
	return d, nil
}
func stringSlice(name string, args []any) (any, error) {
	s := []rune(toString(args[0]))
	count, err := toNumber(args[1])
	if err != nil {
		return nil, err
	}
	n := int(count)
	start := 0
	if name == "RIGHT" {
		start = len(s) - n
	}
	if name == "MID" {
		p, e := toNumber(args[1])
		if e != nil {
			return nil, e
		}
		q, e := toNumber(args[2])
		if e != nil {
			return nil, e
		}
		start = int(p) - 1
		n = int(q)
	}
	if n < 0 {
		return nil, evalFail("negative text length")
	}
	if start < 0 {
		start = 0
	}
	if start > len(s) {
		start = len(s)
	}
	end := start + n
	if end > len(s) {
		end = len(s)
	}
	return string(s[start:end]), nil
}
func formatText(v any, pattern string) (any, error) {
	if s, ok := v.(string); ok {
		if d, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
			return formatDate(d, pattern), nil
		}
		if d, err := time.Parse("2006-01-02", s); err == nil {
			return formatDate(d, pattern), nil
		}
		return s, nil
	}
	f, err := toNumber(v)
	if err != nil {
		return nil, err
	}
	decimals := 0
	if i := strings.Index(pattern, "."); i >= 0 {
		decimals = len(pattern) - i - 1
	}
	formatted := strconv.FormatFloat(f, 'f', decimals, 64)
	if strings.Contains(pattern, ",") {
		parts := strings.SplitN(formatted, ".", 2)
		whole := parts[0]
		sign := ""
		if strings.HasPrefix(whole, "-") {
			sign, whole = "-", whole[1:]
		}
		for i := len(whole) - 3; i > 0; i -= 3 {
			whole = whole[:i] + "," + whole[i:]
		}
		formatted = sign + whole
		if len(parts) == 2 {
			formatted += "." + parts[1]
		}
	}
	first, last := -1, -1
	for i, r := range pattern {
		if strings.ContainsRune("0#,.", r) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first >= 0 {
		digits := len(strings.TrimLeft(strings.SplitN(pattern, ".", 2)[0], "#,"))
		whole, frac, has := strings.Cut(formatted, ".")
		for len(strings.ReplaceAll(whole, ",", "")) < digits {
			whole = "0" + whole
		}
		formatted = whole
		if has {
			formatted += "." + frac
		}
		return pattern[:first] + formatted + pattern[last+1:], nil
	}
	return formatted, nil
}

func formatDate(d time.Time, pattern string) string {
	ampm := "午前"
	if d.Hour() >= 12 {
		ampm = "午後"
	}
	pattern = strings.ReplaceAll(pattern, "AM/PM", "__AMPM__")
	repl := strings.NewReplacer("yyyy", "2006", "YY", "06", "yy", "06", "mmmmm", "1", "mmmm", "January", "mmm", "Jan", "mm", "01", "m", "1", "dddd", "Monday", "ddd", "Mon", "dd", "02", "d", "2", "HH", "15", "hh", "03", "H", "15", "h", "3", "ss", "05", "s", "5", "__AMPM__", ampm)
	return d.Format(repl.Replace(pattern))
}
func jsonLookup(v any, path string) (any, error) {
	raw := toString(v)
	var cur any
	if err := json.Unmarshal([]byte(raw), &cur); err != nil {
		return nil, evalFail("invalid JSON")
	}
	if !strings.HasPrefix(path, "$") {
		return nil, evalFail("invalid JSONPath")
	}
	re := regexp.MustCompile(`(?:\.([A-Za-z0-9_]+)|\[([0-9]+)\])`)
	matches := re.FindAllStringSubmatch(path, -1)
	matchedPath := strings.Builder{}
	for _, m := range matches {
		matchedPath.WriteString(m[0])
		if m[1] != "" {
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil, evalFail("JSONPath object expected")
			}
			cur, ok = obj[m[1]]
			if !ok {
				return nil, evalFail("JSONPath value not found")
			}
		} else {
			arr, ok := cur.([]any)
			if !ok {
				return nil, evalFail("JSONPath array expected")
			}
			i, _ := strconv.Atoi(m[2])
			if i < 0 || i >= len(arr) {
				return nil, evalFail("JSONPath index out of range")
			}
			cur = arr[i]
		}
	}
	if matchedPath.String() != path[1:] {
		return nil, evalFail("unsupported JSONPath")
	}
	if obj, ok := cur.(map[string]any); ok {
		encoded, err := json.Marshal(obj)
		if err != nil {
			return nil, evalFail("encode JSONPath result")
		}
		return string(encoded), nil
	}
	if arr, ok := cur.([]any); ok {
		encoded, err := json.Marshal(arr)
		if err != nil {
			return nil, evalFail("encode JSONPath result")
		}
		return string(encoded), nil
	}
	return cur, nil
}

func flatten(args []any) []any {
	out := []any{}
	for _, a := range args {
		if list, ok := a.([]any); ok {
			out = append(out, list...)
		} else {
			out = append(out, a)
		}
	}
	return out
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

func aggregate(name string, args []any) (any, error) {
	nums := []float64{}
	for _, v := range flatten(args) {
		if isEmpty(v) {
			continue
		}
		f, err := toNumber(v)
		if err != nil {
			return nil, err
		}
		nums = append(nums, f)
	}
	if name == "COUNT" {
		return float64(len(nums)), nil
	}
	if len(nums) == 0 {
		return 0.0, nil
	}
	acc := nums[0]
	sum := 0.0
	for _, f := range nums {
		sum += f
		switch name {
		case "MIN":
			acc = math.Min(acc, f)
		case "MAX":
			acc = math.Max(acc, f)
		}
	}
	switch name {
	case "SUM":
		return sum, nil
	case "AVG", "AVERAGE":
		return sum / float64(len(nums)), nil
	}
	return acc, nil
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s != "" && s != "false" && s != "0"
	case []any:
		return len(x) > 0
	}
	f, err := toNumber(v)
	return err == nil && f != 0
}

func toNumber(v any) (float64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, evalFail("%q is not a number", x)
		}
		return f, nil
	case []any:
		if len(x) == 1 {
			return toNumber(x[0])
		}
	}
	return 0, evalFail("value is not a number")
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(clean(x), 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = toString(e)
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}
