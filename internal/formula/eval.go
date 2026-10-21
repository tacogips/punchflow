package formula

import (
	"fmt"
	"math"
	"strconv"
	"strings"
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
	"ROUND": {1, 2}, "ROUNDUP": {1, 2}, "ROUNDDOWN": {1, 2}, "FLOOR": {1, 1}, "CEIL": {1, 1}, "CEILING": {1, 1}, "ABS": {1, 1},
	"IF": {3, 3}, "AND": {1, -1}, "OR": {1, -1}, "NOT": {1, 1}, "CONCAT": {1, -1},
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
		return math.Floor(x), nil
	case "CEIL", "CEILING":
		return math.Ceil(x), nil
	}
	return math.Abs(x), nil
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
