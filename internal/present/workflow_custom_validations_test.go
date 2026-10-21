package present

import (
	"testing"

	"punchflow/internal/model"
)

func TestWorkflowCustomValidationsArePresented(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	if _, err := q.ExecContext(ctx, `INSERT INTO general_masters(id,code,name,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description,created_at,updated_at) VALUES('gm-custom-validation','custom-validation-master','Custom Validation Master','code',0,0,0,0,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO general_master_items(id,general_master_id,code,name,inputs,input_ids,created_at,updated_at) VALUES('item-custom-validation','gm-custom-validation','cc-eng','Engineering','{}','{}',0,0)`); err != nil {
		t.Fatal(err)
	}
	formValue := "法人"
	formFields := map[string]model.FormField{
		"kind": {ID: "form-kind", Code: "kind", Title: "Account type", FieldType: "pull_down", Size: "full"},
		"listed": {ID: "form-listed", Code: "listed", Title: "Listed", FieldType: "text", Size: "full", CustomValidations: []model.FormCustomValidation{{
			ID: "validation-form", CombinationType: "all", ErrorMessage: "Required for corporations", Fields: []model.FormCustomValidationField{{ID: "condition-form", FormFieldCode: "kind", Symbol: "equal", Value: &formValue}, {ID: "condition-master", FormFieldCode: "kind", Symbol: "equal", GeneralMasterCode: "custom-validation-master", GeneralMasterItemCode: "cc-eng"}, {ID: "condition-team", FormFieldCode: "kind", Symbol: "descendants_or_equal", TeamCode: "ROOT"}},
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
	formConditions := formValidation["fields"].([]any)
	if formConditions[1].(map[string]any)["generalMasterItem"].(map[string]any)["code"] != "cc-eng" || formConditions[2].(map[string]any)["team"].(TeamJSON)["code"] != "ROOT" {
		t.Fatalf("form resource conditions=%v", formConditions[1:])
	}

	detailValue := "3"
	slipFields := []model.SlipField{
		{ID: "slip-qty", Code: "qty", Title: "Quantity", FieldType: "number"},
		{ID: "slip-item", Code: "item", Title: "Item", FieldType: "text", CustomValidations: []model.SlipCustomValidation{{
			ID: "validation-slip", CombinationType: "any", ErrorMessage: "Quantity is too high", Fields: []model.SlipCustomValidationField{{ID: "condition-slip", SlipFieldCode: "qty", Symbol: "greater_than", Value: &detailValue}, {ID: "condition-slip-team", SlipFieldCode: "qty", Symbol: "descendants_or_equal", TeamCode: "ROOT"}},
		}}},
	}
	slip := pf08SlipFieldWithCustomValidations(ctx, q, slipFields[1], slipFields)
	slipValidation := slip["customValidations"].([]any)[0].(map[string]any)
	slipCondition := slipValidation["fields"].([]any)[0].(map[string]any)
	if slipValidation["combinationType"] != "any" || slipCondition["slipField"].(map[string]any)["id"] != "slip-qty" || slipCondition["value"] != &detailValue {
		t.Fatalf("slip validation=%v, condition=%v", slipValidation, slipCondition)
	}
	resource := slipValidation["fields"].([]any)[1].(map[string]any)
	if resource["team"].(TeamJSON)["code"] != "ROOT" {
		t.Fatalf("slip resource condition=%v", resource)
	}
}
