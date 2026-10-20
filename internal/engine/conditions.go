package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"punchflow/internal/model"
)

func pf14InputValues(def model.WorkflowDef, inputs []FieldInput) map[string]any {
	byID, byCode := map[string]any{}, map[string]any{}
	for _, input := range inputs {
		var value any
		if len(input.Value) > 0 {
			_ = json.Unmarshal(input.Value, &value)
		}
		if len(input.GeneralMasterItemID) > 0 {
			_ = json.Unmarshal(input.GeneralMasterItemID, &value)
		}
		if len(input.UserID) > 0 {
			_ = json.Unmarshal(input.UserID, &value)
		}
		if len(input.TeamID) > 0 {
			_ = json.Unmarshal(input.TeamID, &value)
		}
		if len(input.TicketID) > 0 {
			_ = json.Unmarshal(input.TicketID, &value)
		}
		byID[input.FormFieldID] = value
		byCode[input.FormFieldCode] = value
	}
	values := make(map[string]any, len(byID)+len(byCode))
	for k, v := range byID {
		values[k] = v
	}
	for k, v := range byCode {
		if k != "" {
			values[k] = v
		}
	}
	_ = def
	return values
}

func pf14ConditionMatch(fields []model.ConditionField, combination string, values map[string]any) bool {
	if len(fields) == 0 {
		return true
	}
	matched := 0
	for _, field := range fields {
		actual, ok := values[field.FieldKey]
		if !ok {
			actual, ok = values[field.Variable]
		}
		if ok && pf14Compare(actual, field.Symbol, field.Value) {
			matched++
		}
	}
	if combination == model.StepCombinationAny || combination == model.RouteCombinationAny {
		return matched > 0
	}
	return matched == len(fields)
}

func pf14Compare(actual any, symbol string, expected any) bool {
	a, b := fmt.Sprint(actual), fmt.Sprint(expected)
	switch strings.ToLower(symbol) {
	case "equal", "equals", "=", "==":
		return reflect.DeepEqual(actual, expected) || a == b
	case "not_equal", "not_equals", "!=", "<>":
		return !(reflect.DeepEqual(actual, expected) || a == b)
	case "contains":
		return strings.Contains(a, b)
	case "greater_than", ">":
		af, av := pf14Float(actual)
		bf, bv := pf14Float(expected)
		return af && bf && av > bv
	case "greater_than_or_equal", ">=":
		af, av := pf14Float(actual)
		bf, bv := pf14Float(expected)
		return af && bf && av >= bv
	case "less_than", "<":
		af, av := pf14Float(actual)
		bf, bv := pf14Float(expected)
		return af && bf && av < bv
	case "less_than_or_equal", "<=":
		af, av := pf14Float(actual)
		bf, bv := pf14Float(expected)
		return af && bf && av <= bv
	case "is_true":
		return a == "true"
	case "is_false":
		return a == "false"
	default:
		return false
	}
}

func pf14Float(v any) (bool, float64) {
	f, e := strconv.ParseFloat(fmt.Sprint(v), 64)
	return e == nil, f
}

func pf14SectionVisible(section model.WorkflowSection, values map[string]any) bool {
	if section.SectionType != model.SectionTypeForm || !section.Conditional {
		return true
	}
	return pf14ConditionMatch(section.ConditionFields, section.CombinationType, values)
}

func pf14FieldValue(input FieldInput) (any, []string, error) {
	refs := []string{}
	for _, raw := range []json.RawMessage{input.GeneralMasterItemID, input.UserID, input.TeamID, input.TicketID} {
		if len(raw) > 0 {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, nil, fmt.Errorf("invalid referenced id: %w", err)
			}
			switch v := value.(type) {
			case string:
				refs = append(refs, v)
			case []any:
				for _, item := range v {
					refs = append(refs, fmt.Sprint(item))
				}
			}
			return value, refs, nil
		}
	}
	if len(input.Value) == 0 {
		return nil, refs, nil
	}
	var value any
	if err := json.Unmarshal(input.Value, &value); err != nil {
		return nil, nil, err
	}
	return value, refs, nil
}
