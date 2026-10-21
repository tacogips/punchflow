package present

import (
	"testing"

	"punchflow/internal/model"
)

// A condition variable written as "$code" or "${code}" must resolve the same
// form field as the bare code, mirroring the engine's variable normalization.
func TestWorkflowConditionFieldNormalizesVariable(t *testing.T) {
	ctx, db := pf08DB(t)
	fields := map[string]model.FormField{
		"amount": {ID: "ff-amount", Code: "amount", Title: "Amount", FieldType: "number"},
	}
	for _, variable := range []string{"amount", "$amount", "${amount}", " $amount "} {
		out := pf08WorkflowConditionField(ctx, db.Q(), model.ConditionField{ID: "c1", Variable: variable, Symbol: ">", Value: float64(10)}, fields)
		field, ok := out["formField"].(map[string]any)
		if !ok || field == nil {
			t.Fatalf("variable %q: formField = %#v, want the amount field", variable, out["formField"])
		}
		if field["code"] != "amount" {
			t.Errorf("variable %q: formField code = %v, want amount", variable, field["code"])
		}
	}
	out := pf08WorkflowConditionField(ctx, db.Q(), model.ConditionField{ID: "c2", Variable: "$missing", Symbol: "=", Value: "x"}, fields)
	if out["formField"] != nil {
		t.Errorf("unknown variable resolved formField %#v", out["formField"])
	}
}
