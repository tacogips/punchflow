package fixtures

import (
	"strings"
	"testing"

	"punchflow/internal/model"
)

func pf12CustomValidationFixture(t *testing.T) *Set {
	t.Helper()
	set, err := Parse(pf06FSSub(t, "testdata/minimal"))
	if err != nil {
		t.Fatal(err)
	}
	fieldValue := "yes"
	set.Workflows[0].SectionList = []model.WorkflowSection{{ID: "b0000000-0000-4000-8000-000000000001", Title: "Form", SectionType: model.SectionTypeForm, FormFields: []model.FormField{
		{ID: "b0000000-0000-4000-8000-000000000002", Code: "trigger", Title: "Trigger", Size: "full"},
		{ID: "b0000000-0000-4000-8000-000000000003", Code: "answer", Title: "Answer", Size: "full", CustomValidations: []model.FormCustomValidation{{
			ID: "b0000000-0000-4000-8000-000000000004", CombinationType: "all", ErrorMessage: "Answer is required when triggered", Fields: []model.FormCustomValidationField{
				{ID: "b0000000-0000-4000-8000-000000000005", FormFieldCode: "trigger", Symbol: "equal", Value: &fieldValue},
			},
		}}},
	}}}
	return set
}

func TestValidateCustomValidationDefinitions(t *testing.T) {
	set := pf12CustomValidationFixture(t)
	if err := Validate(set); err != nil {
		t.Fatalf("valid custom validation fixture: %v", err)
	}
	set.Workflows[0].SectionList[0].FormFields[1].CustomValidations[0].Fields[0].FormFieldCode = "missing"
	err := Validate(set)
	if err == nil || !strings.Contains(err.Error(), ".formFieldCode: unknown form field code") {
		t.Fatalf("unknown condition field error=%v", err)
	}
}

func TestValidateCustomValidationCombinationAndMessage(t *testing.T) {
	set := pf12CustomValidationFixture(t)
	validation := &set.Workflows[0].SectionList[0].FormFields[1].CustomValidations[0]
	validation.CombinationType = "either"
	validation.ErrorMessage = ""
	err := Validate(set)
	if err == nil || !strings.Contains(err.Error(), ".combinationType: must be all or any") || !strings.Contains(err.Error(), ".errorMessage: must not be empty") {
		t.Fatalf("invalid rule error=%v", err)
	}
}
