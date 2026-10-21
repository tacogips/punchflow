package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func TestReferenceValuesAreSavedSnapshotsAndResolvedArraysStayLive(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		field := pf16FormField("selected-center", "master")
		field.GeneralMasterCode = "cost-centers"
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, field)
				break
			}
		}
		ref := pf16SlipField("row-center", "master")
		ref.GeneralMasterCode = "cost-centers"
		pf16Section(def, "lines").SlipFields = append(pf16Section(def, "lines").SlipFields, ref)
	})
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": "100", "purpose": "trip"})
	itemID := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	for i := range inputs {
		if inputs[i].FormFieldCode == "selected-center" {
			inputs[i].GeneralMasterItemID, _ = json.Marshal(itemID)
		}
	}
	row := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})
	refInput := SlipFieldInput{SlipFieldCode: "row-center"}
	refInput.GeneralMasterItemID, _ = json.Marshal(itemID)
	row.Inputs = append(row.Inputs, refInput)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, inputs, []SlipItemInput{row})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, "UPDATE general_master_items SET name='Engineering renamed' WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	detail, err := present.TicketDetailFor(e.ctx, e.db.Q(), id, nil)
	if err != nil {
		t.Fatal(err)
	}
	var formSnapshot, formLive any
	for _, raw := range detail["inputs"].([]any) {
		input := raw.(map[string]any)
		field := input["formField"].(map[string]any)
		if field["code"] == "selected-center" {
			formSnapshot, formLive = input["value"], input["generalMasterItems"]
		}
	}
	if got := formSnapshot.([]any)[0].(map[string]any)["name"]; got != "Engineering" {
		t.Fatalf("saved form value name=%v, want original snapshot", got)
	}
	if got := generalMasterName(t, formLive); got != "Engineering renamed" {
		t.Fatalf("resolved form array name=%v, want current record", got)
	}
	slips := detail["slipItems"].([]any)
	cell := slips[0].(map[string]any)["inputs"].([]any)[3].(map[string]any)
	if got := cell["value"].([]any)[0].(map[string]any)["name"]; got != "Engineering" {
		t.Fatalf("saved detail value name=%v, want original snapshot", got)
	}
	if got := generalMasterName(t, cell["generalMasterItems"]); got != "Engineering renamed" {
		t.Fatalf("resolved detail array name=%v, want current record", got)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, "DELETE FROM general_master_items WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	detail, err = present.TicketDetailFor(e.ctx, e.db.Q(), id, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range detail["inputs"].([]any) {
		input := raw.(map[string]any)
		field := input["formField"].(map[string]any)
		if field["code"] == "selected-center" {
			if got := input["value"].([]any)[0].(map[string]any)["name"]; got != "Engineering" {
				t.Fatalf("deleted master changed saved form value to %v", got)
			}
			if len(input["generalMasterItems"].([]any)) != 0 {
				t.Fatalf("deleted master resolved array=%v, want empty", input["generalMasterItems"])
			}
		}
	}
	cell = detail["slipItems"].([]any)[0].(map[string]any)["inputs"].([]any)[3].(map[string]any)
	if got := cell["value"].([]any)[0].(map[string]any)["name"]; got != "Engineering" {
		t.Fatalf("deleted master changed saved detail value to %v", got)
	}
}

func generalMasterName(t *testing.T, value any) string {
	t.Helper()
	items, ok := value.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("general master items=%#v", value)
	}
	var item map[string]any
	switch typed := items[0].(type) {
	case map[string]any:
		item = typed
	case present.GeneralMasterItemJSON:
		item = map[string]any(typed)
	default:
		t.Fatalf("general master item=%T", items[0])
	}
	name, _ := item["name"].(string)
	return name
}

func TestRestrictedValuesAreEmptyForCustomAndConditionalValidation(t *testing.T) {
	e := pf14Setup(t)
	secretSectionID, conditionSectionID := "00000000-0000-4000-8000-0000000000d1", "00000000-0000-4000-8000-0000000000d2"
	secret := pf16FormField("secret", "text")
	target := pf16FormField("target", "text")
	required := pf16FormField("condition-required", "text")
	required.Required = true
	radio := pf16FormField("condition-radio", "radio_button")
	radio.Options = []string{"yes", "no"}
	secretValue := "private"
	target.CustomValidations = []model.FormCustomValidation{{CombinationType: "all", ErrorMessage: "must not fire", Fields: []model.FormCustomValidationField{{FormFieldCode: "secret", Symbol: "equal", Value: &secretValue}}}}
	def := model.WorkflowDef{SectionList: []model.WorkflowSection{
		{ID: secretSectionID, SectionType: model.SectionTypeForm, FormFields: []model.FormField{secret}},
		{ID: conditionSectionID, SectionType: model.SectionTypeForm, Conditional: true, CombinationType: "all", ConditionFields: []model.ConditionField{{Variable: "secret", Symbol: "equal", Value: "private"}}, FormFields: []model.FormField{target, required, radio}},
	}}
	secretRow := model.TicketInput{FormFieldID: secret.ID, Value: "private"}
	inputs := []FieldInput{pf14Input("target", target.ID, "ok"), pf14Input("condition-radio", radio.ID, "")}
	_, err := pf14NormalizeForm(e.ctx, e.db.Q(), "ticket-test", def, inputs, nil, formOpts{Full: true, Blocked: map[string]bool{secretSectionID: true}, ExistingInputs: []model.TicketInput{secretRow}}, 1)
	if err != nil {
		t.Fatalf("restricted value triggered custom/conditional/radio validation: %v", err)
	}
}

func TestApproverCannotDeleteSavedDetailRowsOnEitherPatch(t *testing.T) {
	for _, endpoint := range []string{"ticket", "form"} {
		t.Run(endpoint, func(t *testing.T) {
			e := pf14Setup(t)
			initialRows := []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})}
			if endpoint == "form" {
				initialRows = append(initialRows, pf16Row("lines", map[string]any{"item": "pad", "qty": 2, "unit-price": 5}))
			}
			id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), initialRows)
			if err != nil {
				t.Fatal(err)
			}
			approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
			if endpoint == "ticket" {
				err = pf16Update(e, approver, id, UpdateInput{SlipItems: &[]SlipItemInput{}})
			} else {
				err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})}})
			}
			if api, ok := kferr.As(err); !ok || api.Code != "validation_failed" {
				t.Fatalf("delete error=%v, want validation_failed", err)
			}
			rows, readErr := store.TicketSlipItems(e.ctx, e.db.Q(), id)
			wantRows := 1
			if endpoint == "form" {
				wantRows = 2
			}
			if readErr != nil || len(rows) != wantRows {
				t.Fatalf("saved rows=%d read error=%v; want unchanged row count %d", len(rows), readErr, wantRows)
			}
		})
	}
}
