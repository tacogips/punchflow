package present

import (
	"testing"

	"punchflow/internal/model"
)

func TestWorkflowCustomValidationsArePresented(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	formValue := "法人"
	formFields := map[string]model.FormField{
		"kind": {ID: "form-kind", Code: "kind", Title: "Account type", FieldType: "pull_down", Size: "full"},
		"listed": {ID: "form-listed", Code: "listed", Title: "Listed", FieldType: "text", Size: "full", CustomValidations: []model.FormCustomValidation{{
			ID: "validation-form", CombinationType: "all", ErrorMessage: "Required for corporations", Fields: []model.FormCustomValidationField{{ID: "condition-form", FormFieldCode: "kind", Symbol: "equal", Value: &formValue}},
		}}},
	}
	form := pf08FormFieldWithCustomValidations(ctx, q, formFields["listed"], formFields)
	formValidation := form["customValidations"].([]any)[0].(map[string]any)
	if formValidation["combinationType"] != "all" || formValidation["errorMessage"] != "Required for corporations" {
		t.Fatalf("form validation=%v", formValidation)
	}
	formCondition := formValidation["fields"].([]any)[0].(map[string]any)
	if formCondition["formField"].(map[string]any)["id"] != "form-kind" || formCondition["symbol"] != "equal" || formCondition["value"] != &formValue {
		t.Fatalf("form condition=%v", formCondition)
	}

	detailValue := "3"
	slipFields := []model.SlipField{
		{ID: "slip-qty", Code: "qty", Title: "Quantity", FieldType: "number"},
		{ID: "slip-item", Code: "item", Title: "Item", FieldType: "text", CustomValidations: []model.SlipCustomValidation{{
			ID: "validation-slip", CombinationType: "any", ErrorMessage: "Quantity is too high", Fields: []model.SlipCustomValidationField{{ID: "condition-slip", SlipFieldCode: "qty", Symbol: "greater_than", Value: &detailValue}},
		}}},
	}
	slip := pf08SlipFieldWithCustomValidations(slipFields[1], slipFields)
	slipValidation := slip["customValidations"].([]any)[0].(map[string]any)
	slipCondition := slipValidation["fields"].([]any)[0].(map[string]any)
	if slipValidation["combinationType"] != "any" || slipCondition["slipField"].(map[string]any)["id"] != "slip-qty" || slipCondition["value"] != &detailValue {
		t.Fatalf("slip validation=%v, condition=%v", slipValidation, slipCondition)
	}
}
