package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/model"
)

func TestApplicantPartialEditAndReapplicationPreserveApproverValue(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "approver-note" {
					def.SectionList[i].FormFields[j].Author = false
				}
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE ticket_inputs SET value=? WHERE ticket_id=? AND form_field_id=?`, `"reviewed"`, id, fields["approver-note"]); err != nil {
		t.Fatal(err)
	}
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("amount", fields["amount"], "150")}}); err != nil {
		t.Fatalf("unrelated partial edit rejected: %v", err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET status='rejected' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	status := model.TicketStatusInProgress
	inputs := pf16Inputs(t, e, map[string]any{"amount": "200", "purpose": "travel", "approver-note": nil})
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: &status, Inputs: &inputs}); err != nil {
		t.Fatalf("reapplication rejected stored approver-only input: %v", err)
	}
	if got := pf16FormValue(t, e, id, "approver-note"); got != "reviewed" {
		t.Fatalf("stored approver note=%v, want reviewed", got)
	}
}

func TestApproverCalculationDependencyKeepsApplicantOnlyDetailResult(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].SlipFields {
				if def.SectionList[i].SlipFields[j].Code == "subtotal" {
					def.SectionList[i].SlipFields[j].Author = true
					def.SectionList[i].SlipFields[j].Approver = false
				}
			}
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				calc := pf16FormField("approver-lines-total", "calculation")
				calc.Author = false
				calc.Approver = true
				calc.Formula = pf16Formula("SUM(lines.subtotal)")
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, calc)
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})})
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	if err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 2, "unit-price": 10})}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "approver-lines-total"); got != 10.0 {
		t.Fatalf("dependency-driven total=%v, want stored applicant result 10", got)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if got := pf16Cell(t, def, rows[0], "subtotal"); got != 10.0 {
		t.Fatalf("applicant-only detail result changed to %v", got)
	}
}

func TestCalculationCannotReadConditionallyHiddenSection(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, pf16FormField("hidden-total", "calculation"))
			}
		}
		secret := pf16FormField("secret", "number")
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: "conditional-secret", Code: "secret-section", SectionType: model.SectionTypeForm,
			Conditional: true, CombinationType: "all",
			ConditionFields: []model.ConditionField{{Variable: "purpose", Symbol: "equal", Value: "show-secret"}},
			FormFields:      []model.FormField{secret},
		})
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "hidden-total" {
					def.SectionList[i].FormFields[j].Formula = pf16Formula("$secret * 2")
				}
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "travel", "secret": "25"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "hidden-total"); got != nil {
		t.Fatalf("hidden section calculation=%v, want unavailable/null", got)
	}
}

func TestCalculationVisibilityResolvesMasterConditionBeforeFormulaSerialization(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, pf16FormField("hidden-total", "calculation"))
			}
		}
		secret := pf16FormField("secret", "number")
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: "conditional-master-secret", Code: "master-secret-section", SectionType: model.SectionTypeForm,
			Conditional: true, CombinationType: "all",
			ConditionFields: []model.ConditionField{{
				FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "cost-center", Symbol: "not_equal",
				GeneralMasterCode: "cost-centers", GeneralMasterItemCode: "cc-eng",
			}}, FormFields: []model.FormField{secret},
		})
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "hidden-total" {
					def.SectionList[i].FormFields[j].Formula = pf16Formula("$secret * 2")
				}
			}
		}
	})
	itemID := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	inputs := pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "travel", "secret": "25", "cost-center": itemID})
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	encoded, err := json.Marshal(itemID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range inputs {
		if inputs[i].FormFieldID == fields["cost-center"] {
			inputs[i].GeneralMasterItemID = encoded
			inputs[i].Value = nil
		}
	}
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "hidden-total"); got != nil {
		t.Fatalf("calculation from hidden master-conditioned section=%v, want unavailable/null", got)
	}
}

func TestApplicantReapplicationPreservesApproverOnlyDetailCells(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		lines := pf16Section(def, "lines")
		field := pf16SlipField("approver-note", "text")
		field.Author, field.Approver = false, true
		lines.SlipFields = append(lines.SlipFields, field)
	})
	row := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10, "approver-note": nil})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), []SlipItemInput{row})
	if err != nil {
		t.Fatal(err)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if len(rows) != 1 {
		t.Fatalf("detail rows=%d, want 1", len(rows))
	}
	var noteID string
	for _, field := range pf16Section(&def, "lines").SlipFields {
		if field.Code == "approver-note" {
			noteID = field.ID
		}
	}
	if noteID == "" {
		t.Fatal("approver-only detail field not found")
	}
	rows[0].Inputs[noteID] = "reviewed"
	raw, err := json.Marshal(rows[0].Inputs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE ticket_slip_items SET inputs=? WHERE id=?`, string(raw), rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET status='rejected' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	status := model.TicketStatusInProgress
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Status: &status}); err != nil {
		t.Fatalf("reapplication rejected stored approver-only detail cell: %v", err)
	}
	rows = pf16Slips(t, e, id)
	if got := pf16Cell(t, def, rows[0], "approver-note"); got != "reviewed" {
		t.Fatalf("stored approver-only cell=%v, want reviewed", got)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET status='rejected' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	unauthorized := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10, "approver-note": "overwritten"})
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{SlipItems: &[]SlipItemInput{unauthorized}}); err == nil {
		t.Fatal("applicant explicit approver-only detail write was accepted")
	}
}

func TestNumericInputsAreRoundedToFourteenSignificantDigits(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		field := pf16FormField("precision-check", "number")
		field.Author, field.Approver = true, true
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, field)
				return
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "trip", "precision-check": "123456789012345"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "precision-check"); got != "123456789012350" {
		t.Fatalf("stored value=%v (%T), want rounded 14-digit string", got, got)
	}
}

func TestFileCardinalityIsIndependentOfMultipleFlag(t *testing.T) {
	e := pf14Setup(t)
	pf16InsertFile(t, e, "sig-one")
	pf16InsertFile(t, e, "sig-two")
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "receipt" {
					def.SectionList[i].FormFields[j].AllowedExtensions = []string{"txt"}
				}
			}
		}
	})
	inputs := pf16Inputs(t, e, pf16Base, "receipt")
	inputs = append(inputs, pf16FileInput(t, e, "receipt", "sig-one", "sig-two"))
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil)
	if err != nil {
		t.Fatalf("two valid attachments rejected when multiple is false: %v", err)
	}
	got := pf16Attachments(t, e, id)["receipt"]
	if len(got) != 2 {
		t.Fatalf("receipt attachments=%v, want two files", got)
	}
}

func TestMultipleReferenceCardinalityCapsAtTen(t *testing.T) {
	e := pf14Setup(t)
	field := model.FormField{FieldType: "master", Multiple: true}
	if err := pf14cValidateFormValue(e.ctx, e.db.Q(), field, []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven"}); err == nil {
		t.Fatal("11 reference selections accepted; want maximum 10")
	}
	field.FieldType = "file"
	tooManyFiles := make([]string, 101)
	if err := pf14cValidateFormValue(e.ctx, e.db.Q(), field, tooManyFiles); err == nil {
		t.Fatal("101 file attachments accepted; want maximum 100")
	}
}
