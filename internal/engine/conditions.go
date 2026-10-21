package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"punchflow/internal/conditions"
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
	return conditions.Match(fields, combination, values)
}

func pf14Compare(actual any, symbol string, expected any) bool {
	return conditions.Compare(actual, symbol, expected)
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
