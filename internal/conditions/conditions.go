// Package conditions evaluates value-based workflow and section conditions.
package conditions

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"punchflow/internal/model"
)

// Value resolves a condition variable against normalized and literal map keys.
func Value(values map[string]any, variable string) (any, bool) {
	name := model.VariableName(variable)
	if value, ok := values[name]; ok {
		return value, true
	}
	value, ok := values[variable]
	return value, ok
}

// Match evaluates condition fields with the engine's comparison semantics.
func Match(fields []model.ConditionField, combination string, values map[string]any) bool {
	if len(fields) == 0 {
		return true
	}
	matched := 0
	for _, field := range fields {
		key := field.Variable
		if key == "" {
			key = field.FieldKey
		}
		actual, ok := Value(values, key)
		if ok && Compare(actual, field.Symbol, field.Value) {
			matched++
		}
	}
	if combination == model.StepCombinationAny || combination == model.RouteCombinationAny {
		return matched > 0
	}
	return matched == len(fields)
}

// Compare applies the supported value comparison operators.
func Compare(actual any, symbol string, expected any) bool {
	a, b := fmt.Sprint(actual), fmt.Sprint(expected)
	switch strings.ToLower(symbol) {
	case "equal", "equals", "=", "==":
		return reflect.DeepEqual(actual, expected) || a == b
	case "not_equal", "not_equals", "!=", "<>":
		return !reflect.DeepEqual(actual, expected) && a != b
	case "contains":
		return strings.Contains(a, b)
	case "include", "exclude":
		found := false
		switch values := actual.(type) {
		case []any:
			for _, value := range values {
				if reflect.DeepEqual(value, expected) || fmt.Sprint(value) == b {
					found = true
					break
				}
			}
		case []string:
			for _, value := range values {
				if value == b {
					found = true
					break
				}
			}
		default:
			found = reflect.DeepEqual(actual, expected) || a == b
		}
		if strings.ToLower(symbol) == "exclude" {
			return !found
		}
		return found
	case "is_empty":
		return actual == nil || a == "" || emptyArray(actual)
	case "is_not_empty":
		return actual != nil && a != "" && !emptyArray(actual)
	case "descendants_or_equal":
		return reflect.DeepEqual(actual, expected) || a == b
	case "not_descendants_or_equal":
		return !reflect.DeepEqual(actual, expected) && a != b
	case "greater_than", ">":
		av, aok := number(actual)
		bv, bok := number(expected)
		return aok && bok && av > bv
	case "greater_than_or_equal", ">=":
		av, aok := number(actual)
		bv, bok := number(expected)
		return aok && bok && av >= bv
	case "less_than", "<":
		av, aok := number(actual)
		bv, bok := number(expected)
		return aok && bok && av < bv
	case "less_than_or_equal", "<=":
		av, aok := number(actual)
		bv, bok := number(expected)
		return aok && bok && av <= bv
	case "is_true":
		return a == "true"
	case "is_false":
		return a == "false"
	default:
		return false
	}
}

func emptyArray(value any) bool {
	rv := reflect.ValueOf(value)
	return (rv.Kind() == reflect.Array || rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.Len() == 0
}

func number(value any) (float64, bool) {
	f, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	return f, err == nil
}
