package engine

import (
	"punchflow/internal/conditions"
	"punchflow/internal/model"
)

// validateCustomFields evaluates configured validations against the complete
// normalized form and detail rows. A matching rule adds its configured error
// message to the field the rule belongs to.
func (n *normalizer) validateCustomFields(inputs []model.TicketInput, slips []model.TicketSlipItem) {
	formValues := make(map[string]any, len(inputs)*2)
	for _, input := range inputs {
		if field, ok := n.idx.fieldByID[input.FormFieldID]; ok {
			if !n.opts.Blocked[n.idx.fieldSection[field.ID]] {
				formValues[input.FormFieldID] = input.Value
				formValues[field.Code] = input.Value
			}
		}
	}
	for _, section := range n.def.SectionList {
		for _, field := range section.FormFields {
			if n.opts.Blocked[section.ID] {
				formValues[field.Code], formValues[field.ID] = nil, nil
				continue
			}
			if value, ok := formValues[field.Code]; ok {
				formValues[field.ID] = value
			}
			for _, validation := range field.CustomValidations {
				if pf12CustomValidationMatches(validation.CombinationType, validation.Fields, func(code string) (any, bool) {
					value, ok := formValues[code]
					return value, ok
				}) {
					n.fail(field.Code, validation.ErrorMessage)
				}
			}
		}
	}
	sections := make(map[string]model.WorkflowSection, len(n.idx.slipByID))
	for _, section := range n.def.SectionList {
		if section.SectionType == model.SectionTypeSlip {
			sections[section.ID] = section
		}
	}
	for _, row := range slips {
		section, ok := sections[row.SlipSectionID]
		if !ok {
			continue
		}
		values := make(map[string]any, len(section.SlipFields)*2)
		for _, field := range section.SlipFields {
			value, ok := row.Inputs[field.ID]
			if !ok {
				value, ok = row.Inputs[field.Code]
			}
			if ok {
				values[field.ID], values[field.Code] = value, value
			}
		}
		for _, field := range section.SlipFields {
			for _, validation := range field.CustomValidations {
				if pf12SlipCustomValidationMatches(validation.CombinationType, validation.Fields, func(code string) (any, bool) {
					value, ok := values[code]
					return value, ok
				}) {
					n.fail(field.Code, validation.ErrorMessage)
				}
			}
		}
	}
}

func pf12CustomValidationMatches(combination string, fields []model.FormCustomValidationField, value func(string) (any, bool)) bool {
	conditions := make([]model.ConditionField, 0, len(fields))
	for _, field := range fields {
		var expected any
		if field.Value != nil {
			expected = *field.Value
		}
		conditions = append(conditions, model.ConditionField{Variable: field.FormFieldCode, Symbol: field.Symbol, Value: expected})
	}
	return pf12MatchConditions(conditions, combination, value)
}

func pf12SlipCustomValidationMatches(combination string, fields []model.SlipCustomValidationField, value func(string) (any, bool)) bool {
	conditions := make([]model.ConditionField, 0, len(fields))
	for _, field := range fields {
		var expected any
		if field.Value != nil {
			expected = *field.Value
		}
		conditions = append(conditions, model.ConditionField{Variable: field.SlipFieldCode, Symbol: field.Symbol, Value: expected})
	}
	return pf12MatchConditions(conditions, combination, value)
}

func pf12MatchConditions(fields []model.ConditionField, combination string, value func(string) (any, bool)) bool {
	if len(fields) == 0 {
		return false
	}
	matched := 0
	for _, field := range fields {
		actual, _ := value(field.Variable)
		if conditions.Compare(actual, field.Symbol, field.Value) {
			matched++
		}
	}
	if combination == "any" {
		return matched > 0
	}
	return matched == len(fields)
}
