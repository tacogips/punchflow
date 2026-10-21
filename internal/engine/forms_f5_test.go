package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestApproverFormPatchPreservesOmittedDetailSections(t *testing.T) {
	for _, endpoint := range []string{"ticket", "form"} {
		t.Run(endpoint, func(t *testing.T) {
			e := pf14Setup(t)
			pf16TwoSections(t, e)
			id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{
				pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
				pf16Row("extras", map[string]any{"memo": "preserve me"}),
			})
			if err != nil {
				t.Fatal(err)
			}
			approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
			rowsToPatch := []SlipItemInput{pf16Row("lines", map[string]any{"item": "pencil", "qty": 2, "unit-price": 20, "reviewer-note": nil})}
			if endpoint == "ticket" {
				err = pf16Update(e, approver, id, UpdateInput{SlipItems: &rowsToPatch})
			} else {
				err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: rowsToPatch})
			}
			if err != nil {
				t.Fatalf("partial approver update: %v", err)
			}
			def := pf16Def(t, e, "expense-claim")
			rows := pf16Slips(t, e, id)
			if len(rows) != 2 {
				t.Fatalf("rows after partial update=%d, want 2", len(rows))
			}
			for _, row := range rows {
				if pf16Section(&def, "extras").ID == row.SlipSectionID && pf16Cell(t, def, row, "memo") != "preserve me" {
					t.Fatalf("omitted section row was changed: %+v", row)
				}
			}
			empty := []SlipItemInput{}
			if endpoint == "ticket" {
				err = pf16Update(e, approver, id, UpdateInput{SlipItems: &empty})
			} else {
				err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: empty})
			}
			if err == nil {
				t.Fatal("explicit section replacement must not delete saved approver rows")
			}
		})
	}
}

func TestMalformedDetailRowsAreRejectedWithoutErasingSavedCells(t *testing.T) {
	e := pf14Setup(t)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
	})
	if err != nil {
		t.Fatal(err)
	}
	badRows := []SlipItemInput{{SlipSectionCode: "lines"}}
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: badRows}); err == nil {
		t.Fatal("row without inputs was accepted")
	}
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pencil", "unit-price": 20}),
	}}); err == nil {
		t.Fatal("row missing an editable cell was accepted")
	}
	rows := pf16Slips(t, e, id)
	def := pf16Def(t, e, "expense-claim")
	if len(rows) != 1 || pf16Cell(t, def, rows[0], "item") != "pen" || pf16Cell(t, def, rows[0], "qty") != 1.0 {
		t.Fatalf("malformed patch changed saved cells: %+v", rows)
	}
}

func TestSelectedUserSnapshotsExposeOnlyPublicCustomFields(t *testing.T) {
	e := pf14Setup(t)
	admin, err := store.GetUser(e.ctx, e.db.Q(), e.user("admin"))
	if err != nil {
		t.Fatal(err)
	}
	n := normalizer{ctx: e.ctx, q: e.db.Q()}
	snapshot, err := n.referenceSnapshot("user", []string{admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	fields := snapshot.([]any)[0].(map[string]any)["customFields"].(map[string]any)
	if fields["cost-center"] != "CC-100" {
		t.Fatalf("public custom field missing from user snapshot: %#v", fields)
	}
	if _, ok := fields["hire-date"]; ok {
		t.Fatalf("private custom field leaked into user snapshot: %#v", fields)
	}
	encoded, err := calcUserJSON(e.ctx, e.db.Q(), admin)
	if err != nil {
		t.Fatal(err)
	}
	var formulaUser map[string]any
	if err = json.Unmarshal([]byte(encoded), &formulaUser); err != nil {
		t.Fatal(err)
	}
	formulaFields := formulaUser["customFields"].(map[string]any)
	if _, ok := formulaFields["hire-date"]; ok {
		t.Fatalf("private custom field leaked into formula metadata: %#v", formulaFields)
	}
}

func TestSubmissionRefreshesDetailAndApplicantTeamSnapshots(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		field := pf16SlipField("row-center", "master")
		field.GeneralMasterCode = "cost-centers"
		pf16Section(def, "lines").SlipFields = append(pf16Section(def, "lines").SlipFields, field)
	})
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	itemID := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	row := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})
	ref := SlipFieldInput{SlipFieldCode: "row-center"}
	ref.GeneralMasterItemID, _ = json.Marshal(itemID)
	row.Inputs = append(row.Inputs, ref)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16CompleteForTest(t, e, fields), []SlipItemInput{row})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, "UPDATE general_master_items SET name='Renamed before submit' WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, "UPDATE teams SET name='Sales renamed' WHERE id=?", e.team("sales")); err != nil {
		t.Fatal(err)
	}
	status := model.TicketStatusInProgress
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: &status}); err != nil {
		t.Fatalf("submit draft: %v", err)
	}
	ticket := e.ticket(id)
	var teamSnapshot map[string]any
	if err = json.Unmarshal(ticket.AuthorTeamSnapshot, &teamSnapshot); err != nil {
		t.Fatal(err)
	}
	if teamSnapshot["name"] != "Sales renamed" {
		t.Fatalf("applicant team snapshot=%v, want fresh name", teamSnapshot["name"])
	}
	rows := pf16Slips(t, e, id)
	def := pf16Def(t, e, "expense-claim")
	rowSnapshot := rows[0].ReferenceSnapshots[func() string {
		for _, field := range pf16Section(&def, "lines").SlipFields {
			if field.Code == "row-center" {
				return field.ID
			}
		}
		return ""
	}()].([]any)[0].(map[string]any)
	if rowSnapshot["name"] != "Renamed before submit" {
		t.Fatalf("detail reference snapshot=%v, want refreshed item", rowSnapshot["name"])
	}
}

func TestUnchangedHistoricalReferenceDoesNotBlockPartialUpdate(t *testing.T) {
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
	})
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	inputs := pf16Inputs(t, e, pf16Base)
	itemID := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	for i := range inputs {
		if inputs[i].FormFieldCode == "selected-center" {
			inputs[i].GeneralMasterItemID, _ = json.Marshal(itemID)
		}
	}
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, "DELETE FROM general_master_items WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("purpose", fields["purpose"], "unrelated change")}}); err != nil {
		t.Fatalf("unrelated patch rejected historical reference: %v", err)
	}
}

func TestRouteBranchingTreatsRestrictedApplicantValuesAsEmpty(t *testing.T) {
	field := pf16FormField("secret", "text")
	section := model.WorkflowSection{ID: "restricted", SectionType: model.SectionTypeForm, ViewRestriction: true, FormFields: []model.FormField{field}}
	route := "secret-route"
	def := model.WorkflowDef{RouteCode: "default", SectionList: []model.WorkflowSection{section}, RouteConditions: []model.RouteCondition{
		{ConditionType: model.RouteConditionTypeField, CombinationType: "all", RouteCode: &route, ConditionFields: []model.ConditionField{{Variable: "secret", Symbol: "equal", Value: "branch"}}},
	}}
	values := map[string]any{"secret": "branch", model.VariableName("secret"): "branch", field.ID: "branch"}
	selected, err := pf14SelectRouteCode(def, pf14RouteApplicantValues(def, values, map[string]bool{"restricted": true}))
	if err != nil {
		t.Fatal(err)
	}
	if selected != "default" {
		t.Fatalf("selected route=%q, want default with hidden applicant value filtered", selected)
	}
	selected, err = pf14SelectRouteCode(def, pf14RouteApplicantValues(def, values, map[string]bool{}))
	if err != nil {
		t.Fatal(err)
	}
	if selected != route {
		t.Fatalf("applicant allowed to view the section selected route=%q, want %q", selected, route)
	}
}

func pf16CompleteForTest(t *testing.T, e *pf14TestEnv, fields map[string]string) []FieldInput {
	t.Helper()
	return pf14CompleteInputs(fields, pf16Base)
}
