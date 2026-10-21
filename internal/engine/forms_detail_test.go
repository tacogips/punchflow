package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

func TestSubmitRevalidatesMasterAndMultipleDetailReferences(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		master := pf16SlipField("master-ref", "master")
		master.GeneralMasterCode = "cost-centers"
		multiple := pf16SlipField("multi-ref", "master")
		multiple.GeneralMasterCode = "cost-centers"
		multiple.Multiple = true
		pf16Section(def, "lines").SlipFields = append(pf16Section(def, "lines").SlipFields, master, multiple)
	})
	first := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	second := pf14cMasterItemID(t, e, "cost-centers", "cc-sales")
	firstJSON, _ := json.Marshal(first)
	multipleJSON, _ := json.Marshal([]string{first, second})
	row := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 1})
	row.Inputs = append(row.Inputs,
		SlipFieldInput{SlipFieldCode: "master-ref", GeneralMasterItemID: firstJSON},
		SlipFieldInput{SlipFieldCode: "multi-ref", GeneralMasterItemID: multipleJSON},
	)
	if _, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{row}); err != nil {
		t.Fatalf("submission rejected valid master references: %v", err)
	}
}

var pf16Base = map[string]any{"amount": "100", "purpose": "trip"}

// pf16TwoSections adds a second detail section and an approver-only column.
func pf16TwoSections(t *testing.T, e *pf14TestEnv) {
	t.Helper()
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		lines := pf16Section(def, "lines")
		lines.SlipFields = append(lines.SlipFields, func() model.SlipField {
			f := pf16SlipField("reviewer-note", "text")
			f.Approver = true
			return f
		}())
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: "slip-extras-section", Code: "extras", Title: "Extras", SectionType: model.SectionTypeSlip,
			SlipFields: []model.SlipField{pf16SlipField("memo", "text")},
		})
	})
}

func TestFormPatchReplacesOnlySuppliedDetailSections(t *testing.T) {
	e := pf14Setup(t)
	pf16TwoSections(t, e)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10, "reviewer-note": "keep me"}),
		pf16Row("extras", map[string]any{"memo": "extra memo"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	def := pf16Def(t, e, "expense-claim")
	before := pf16Slips(t, e, id)
	if len(before) != 2 {
		t.Fatalf("created %d detail rows, want 2", len(before))
	}
	// R03/PF-08: updating one section must neither drop the other section nor
	// the omitted approver-only cell of the submitted row.
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pencil", "qty": 2, "unit-price": 20}),
	}})
	if err != nil {
		t.Fatalf("form patch: %v", err)
	}
	after := pf16Slips(t, e, id)
	if len(after) != 2 {
		t.Fatalf("after patch %d rows, want 2 (other section must survive): %+v", len(after), after)
	}
	linesID := pf16Section(&def, "lines").ID
	extrasID := pf16Section(&def, "extras").ID
	for _, row := range after {
		switch row.SlipSectionID {
		case linesID:
			if got := pf16Cell(t, def, row, "item"); got != "pencil" {
				t.Errorf("lines item=%v want pencil", got)
			}
			if got := pf16Cell(t, def, row, "reviewer-note"); got != "keep me" {
				t.Errorf("omitted approver-only cell=%v want preserved", got)
			}
		case extrasID:
			if got := pf16Cell(t, def, row, "memo"); got != "extra memo" {
				t.Errorf("extras memo=%v want untouched", got)
			}
		}
	}
	// Several rows for the same section replace that section's rows.
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "a", "qty": 1, "unit-price": 1}),
		pf16Row("lines", map[string]any{"item": "b", "qty": 2, "unit-price": 2}),
		pf16Row("lines", map[string]any{"item": "c", "qty": 3, "unit-price": 3}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows := pf16Slips(t, e, id)
	if pf16CountSlipRows(rows, linesID) != 3 || pf16CountSlipRows(rows, extrasID) != 1 {
		t.Fatalf("rows lines=%d extras=%d, want 3 and 1", pf16CountSlipRows(rows, linesID), pf16CountSlipRows(rows, extrasID))
	}
	// PATCH /tickets keeps its replace-all contract for slipItems.
	err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{SlipItems: &[]SlipItemInput{pf16Row("lines", map[string]any{"item": "only", "qty": 1, "unit-price": 1})}})
	if err != nil {
		t.Fatal(err)
	}
	if rows = pf16Slips(t, e, id); len(rows) != 1 {
		t.Fatalf("PATCH ticket kept %d rows, want 1", len(rows))
	}
}

func TestDetailSectionDefaultsToFirst(t *testing.T) {
	e := pf14Setup(t)
	pf16TwoSections(t, e)
	def := pf16Def(t, e, "expense-claim")
	// R09: neither slipSectionId nor slipSectionCode -> first detail section.
	row := pf16Row("", map[string]any{"item": "pen", "qty": 1, "unit-price": 5})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{row})
	if err != nil {
		t.Fatalf("create without selectors: %v", err)
	}
	rows := pf16Slips(t, e, id)
	if len(rows) != 1 || rows[0].SlipSectionID != pf16Section(&def, "lines").ID {
		t.Fatalf("rows=%+v, want one row in the first detail section", rows)
	}
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{SlipItems: &[]SlipItemInput{row}}); err != nil {
		t.Fatalf("update without selectors: %v", err)
	}
	// ... but the /form contract still requires an explicit selector.
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{row}})
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
		t.Fatalf("form patch without selector error=%v, want invalid_parameter", err)
	}
}

func TestDetailFieldsShareFormValidation(t *testing.T) {
	e := pf14Setup(t)
	good := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})
	// R08/PF-17: type validation on detail cells.
	_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": "abc", "unit-price": 10}),
	})
	assertPF14FieldError(t, err, "validation_failed", "qty")
	_, err = pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1.5, "unit-price": 10}),
	})
	assertPF14FieldError(t, err, "validation_failed", "qty")

	// Omitting a required cell is allowed in a draft but not on submission.
	missing := pf16Row("lines", map[string]any{"item": "pen", "qty": 1})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{missing})
	if err != nil {
		t.Fatalf("draft with incomplete row rejected: %v", err)
	}
	_, err = pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{missing})
	assertPF14FieldError(t, err, "validation_failed", "unit-price")

	// Submitting the saved draft revalidates the stored detail rows.
	err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: ptr(model.TicketStatusInProgress)})
	assertPF14FieldError(t, err, "validation_failed", "unit-price")
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{good}}); err != nil {
		t.Fatal(err)
	}
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: ptr(model.TicketStatusInProgress)}); err != nil {
		t.Fatalf("submit after fixing rows: %v", err)
	}
}

func TestApproverDetailEditRestrictions(t *testing.T) {
	e := pf14Setup(t)
	pf16TwoSections(t, e)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) { def.Overwritable = false })
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
	})
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "hacked", "qty": 1, "unit-price": 10})}})
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
		t.Fatalf("approver editing applicant detail cell error=%v, want invalid_parameter", err)
	}
	// Only approver cells are editable; the applicant cells are preserved.
	err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"reviewer-note": "ok"})}})
	if err != nil {
		t.Fatalf("approver editing approver cell: %v", err)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if len(rows) != 1 || pf16Cell(t, def, rows[0], "reviewer-note") != "ok" || pf16Cell(t, def, rows[0], "item") != "pen" {
		t.Fatalf("rows=%+v, want reviewer-note set and item preserved", rows)
	}
}

func TestPartialFormRejectsDuplicateFieldReferences(t *testing.T) {
	e := pf14Setup(t)
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	id := e.createExpense("sales-rep", 10, false)
	// R22: the same field by id and by code (or twice by id) is a 400.
	for name, patch := range map[string][]FieldInput{
		"id and code": {pf14Input("", fields["purpose"], "one"), pf14Input("purpose", "", "two")},
		"twice by id": {pf14Input("", fields["purpose"], "one"), pf14Input("", fields["purpose"], "two")},
	} {
		t.Run(name, func(t *testing.T) {
			err := pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: patch})
			if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" || api.Status != 400 {
				t.Fatalf("duplicate patch error=%v, want 400 invalid_parameter", err)
			}
		})
	}
	for _, row := range mustTicketInputs(t, e, id) {
		if row.FormFieldID == fields["purpose"] && fmt.Sprint(row.Value) == "one" || fmt.Sprint(row.Value) == "two" {
			t.Fatalf("rejected duplicate patch was applied: %v", row.Value)
		}
	}
}

func TestRadioButtonMustBeSelectedOnSubmitAndApproverUpdate(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		choice := pf16FormField("choice", "radio_button")
		choice.Options = []string{"a", "b"}
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, choice)
				return
			}
		}
	})
	// Drafts may keep an empty radio button.
	draft, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
	if err != nil {
		t.Fatalf("draft with empty radio: %v", err)
	}
	// Submission requires a selection, even though the field is not required.
	_, err = pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), nil)
	assertPF14FieldError(t, err, "validation_failed", "choice")
	err = pf16Update(e, e.actor("sales-rep"), draft, UpdateInput{Status: ptr(model.TicketStatusInProgress)})
	assertPF14FieldError(t, err, "validation_failed", "choice")

	submitted, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "trip", "choice": "a"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, submitted)}
	// R21: an approver form update may not blank the radio button.
	err = pf16UpdateForm(e, approver, submitted, FormPatch{Inputs: []FieldInput{pf14Input("choice", "", "")}})
	assertPF14FieldError(t, err, "validation_failed", "choice")
	if err = pf16UpdateForm(e, approver, submitted, FormPatch{Inputs: []FieldInput{pf14Input("choice", "", "b")}}); err != nil {
		t.Fatalf("approver selecting a radio value: %v", err)
	}
}

func TestStoredDetailRadioButtonMustBeSelectedAtSubmission(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		section := pf16Section(def, "lines")
		choice := pf16SlipField("slip-choice", "radio_button")
		choice.Options = []string{"a", "b"}
		section.SlipFields = append(section.SlipFields, choice)
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10, "slip-choice": ""}),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: ptr(model.TicketStatusInProgress)})
	assertPF14FieldError(t, err, "validation_failed", "slip-choice")
}

func ptr[T any](v T) *T { return &v }
