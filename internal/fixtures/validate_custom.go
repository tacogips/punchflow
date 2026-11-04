package fixtures

import (
	"fmt"

	"punchflow/internal/model"
)

var pf12CustomValidationSymbols = []string{
	"equal", "not_equal", "greater_than", "greater_than_or_equal", "less_than",
	"less_than_or_equal", "include", "exclude", "is_empty", "is_not_empty", "descendants_or_equal",
}

func pf12CustomValidationProblems(s *Set) []Problem {
	var problems []Problem
	add := func(path, message string) {
		problems = append(problems, Problem{File: FileWorkflows, Path: path, Message: message})
	}
	for wi, workflow := range s.Workflows {
		base := fmt.Sprintf("workflows[%d]", wi)
		formCodes, formCount, slipCount := map[string]bool{}, 0, 0
		for _, section := range workflow.SectionList {
			for _, field := range section.FormFields {
				formCodes[field.Code] = true
				formCount += len(field.CustomValidations)
			}
			for _, field := range section.SlipFields {
				slipCount += len(field.CustomValidations)
			}
		}
		if formCount > 50 {
			add(base+".sectionList", "custom validations exceed the workflow limit of 50 form validations")
		}
		if slipCount > 10 {
			add(base+".sectionList", "custom validations exceed the workflow limit of 10 detail validations")
		}
		for si, section := range workflow.SectionList {
			sectionPath := fmt.Sprintf("%s.sectionList[%d]", base, si)
			slipCodes := map[string]bool{}
			for _, field := range section.SlipFields {
				slipCodes[field.Code] = true
			}
			for fi, field := range section.FormFields {
				path := fmt.Sprintf("%s.formFields[%d].customValidations", sectionPath, fi)
				for vi, validation := range field.CustomValidations {
					pf12ValidateFormCustomValidation(validation, path+fmt.Sprintf("[%d]", vi), formCodes, add)
				}
			}
			for fi, field := range section.SlipFields {
				path := fmt.Sprintf("%s.slipFields[%d].customValidations", sectionPath, fi)
				for vi, validation := range field.CustomValidations {
					vp := path + fmt.Sprintf("[%d]", vi)
					if validation.CombinationType != "all" && validation.CombinationType != "any" {
						add(vp+".combinationType", "must be all or any")
					}
					if validation.ErrorMessage == "" {
						add(vp+".errorMessage", "must not be empty")
					}
					if len(validation.Fields) == 0 {
						add(vp+".fields", "must not be empty")
					}
					for ci, condition := range validation.Fields {
						cp := fmt.Sprintf("%s.fields[%d]", vp, ci)
						if condition.SlipFieldCode == "" || !slipCodes[condition.SlipFieldCode] {
							add(cp+".slipFieldCode", "unknown detail field code")
						}
						pf12ValidateCustomValidationSymbol(condition.Symbol, condition.Value, cp, add)
					}
				}
			}
		}
	}
	return problems
}

func pf12ValidateFormCustomValidation(validation model.FormCustomValidation, path string, formCodes map[string]bool, add func(string, string)) {
	if validation.CombinationType != "all" && validation.CombinationType != "any" {
		add(path+".combinationType", "must be all or any")
	}
	if validation.ErrorMessage == "" {
		add(path+".errorMessage", "must not be empty")
	}
	if len(validation.Fields) == 0 {
		add(path+".fields", "must not be empty")
	}
	for i, condition := range validation.Fields {
		cp := fmt.Sprintf("%s.fields[%d]", path, i)
		if condition.FormFieldCode == "" || !formCodes[condition.FormFieldCode] {
			add(cp+".formFieldCode", "unknown form field code")
		}
		pf12ValidateCustomValidationSymbol(condition.Symbol, condition.Value, cp, add)
	}
}

func pf12ValidateCustomValidationSymbol(symbol string, value *string, path string, add func(string, string)) {
	if !pf06Contains(pf12CustomValidationSymbols, symbol) {
		add(path+".symbol", "invalid custom validation symbol")
	}
	if value == nil && symbol != "is_empty" && symbol != "is_not_empty" {
		add(path+".value", "is required for this symbol")
	}
}
