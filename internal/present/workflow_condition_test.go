package present

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestTicketSectionVisibilityResolvesMasterReferenceThreshold(t *testing.T) {
	ctx, db := pf08DB(t)
	masterID, err := store.ResolveID(ctx, db.Q(), "general_masters", "GM")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := store.MasterItemIDByCode(ctx, db.Q(), masterID, "I1")
	if err != nil {
		t.Fatal(err)
	}
	section := model.WorkflowSection{
		SectionType: model.SectionTypeForm, Conditional: true, CombinationType: model.StepCombinationAll,
		ConditionFields: []model.ConditionField{{
			FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "selection", Symbol: "equal",
			GeneralMasterCode: "GM", GeneralMasterItemCode: "I1",
		}},
	}
	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{{name: "matching item", value: itemID, want: true}, {name: "different item", value: "different-item", want: false}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pf08TicketSectionVisible(ctx, db.Q(), section, map[string]any{"selection": tc.value})
			if err != nil || got != tc.want {
				t.Fatalf("visible=%v err=%v, want %v", got, err, tc.want)
			}
		})
	}
}

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

func TestWorkflowConditionFieldPresentsConfiguredMasterThreshold(t *testing.T) {
	ctx, db := pf08DB(t)
	masterID, err := store.ResolveID(ctx, db.Q(), "general_masters", "GM")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := store.MasterItemIDByCode(ctx, db.Q(), masterID, "I1")
	if err != nil {
		t.Fatal(err)
	}
	out := pf08WorkflowConditionField(ctx, db.Q(), model.ConditionField{
		ID: "c-master", FieldKey: model.StepConditionFieldKeyGeneralMasterVariable,
		Variable: "cost-center", Symbol: "equal", GeneralMasterCode: "GM", GeneralMasterItemCode: "I1",
	}, nil)
	if out["value"] != nil {
		t.Fatalf("master threshold value=%v, want null when reference metadata is configured", out["value"])
	}
	item, ok := out["generalMasterItem"].(GeneralMasterItemJSON)
	if !ok || item["id"] != itemID || item["code"] != "I1" {
		t.Fatalf("generalMasterItem=%#v, want resolved item %s", out["generalMasterItem"], itemID)
	}
}
