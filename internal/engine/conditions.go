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
	idToCode := map[string]string{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			idToCode[field.ID] = field.Code
		}
	}
	explicitCodes := map[string]bool{}
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
		if input.FormFieldCode != "" {
			explicitCodes[input.FormFieldCode] = true
		}
		if input.FormFieldCode == "" && input.FormFieldID != "" {
			if code := idToCode[input.FormFieldID]; code != "" && !explicitCodes[code] {
				byCode[code] = value
			}
		}
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
		return actual == nil || a == "" || pf14bEmptyArray(actual)
	case "is_not_empty":
		return actual != nil && a != "" && !pf14bEmptyArray(actual)
	case "descendants_or_equal":
		return reflect.DeepEqual(actual, expected) || a == b
	case "not_descendants_or_equal":
		return !reflect.DeepEqual(actual, expected) && a != b
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

func pf14bEmptyArray(value any) bool {
	rv := reflect.ValueOf(value)
	return (rv.Kind() == reflect.Array || rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.Len() == 0
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
