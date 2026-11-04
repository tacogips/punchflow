package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestApplicantCannotWriteApproverOnlyFormFields(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "purpose" {
					def.SectionList[i].FormFields[j].Author = false
				}
			}
		}
	})
	inputs := pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": nil})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil)
	if err != nil {
		t.Fatalf("null placeholder for applicant-inaccessible field: %v", err)
	}
	if got := pf16FormValue(t, e, id, "purpose"); got != nil {
		t.Fatalf("inaccessible purpose=%v, want null", got)
	}
	inputs = pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "attempted"})
	if _, err = pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil); err == nil {
		t.Fatal("applicant supplied a non-null approver-only field")
	}

	eExisting := pf14Setup(t)
	existingID, err := pf16Create(eExisting, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, eExisting, map[string]any{"amount": "100", "purpose": "saved before permission change"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	pf16Redefine(t, eExisting, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "purpose" {
					def.SectionList[i].FormFields[j].Author = false
				}
			}
		}
	})
	if err = pf16UpdateForm(eExisting, eExisting.actor("sales-rep"), existingID, FormPatch{Inputs: []FieldInput{pf14Input("purpose", "", nil)}}); err != nil {
		t.Fatalf("null placeholder should preserve an existing inaccessible value: %v", err)
	}
	if got := pf16FormValue(t, eExisting, existingID, "purpose"); got != "saved before permission change" {
		t.Fatalf("inaccessible saved value=%v, want prior value", got)
	}
	if err = pf16UpdateForm(eExisting, eExisting.actor("sales-rep"), existingID, FormPatch{Inputs: []FieldInput{pf14Input("purpose", "", "overwrite")}}); err == nil {
		t.Fatal("applicant updated a now-inaccessible saved field")
	}
}

func TestApproverDetailPartialRowMustSupplyEditableCellsAndNullClears(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		lines := pf16Section(def, "lines")
		for i := range lines.SlipFields {
			lines.SlipFields[i].Author = true
			lines.SlipFields[i].Approver = true
		}
	})
	original := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{original})
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pencil"})}})
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
		t.Fatalf("incomplete approver row error=%v, want invalid_parameter", err)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if len(rows) != 1 || pf16Cell(t, def, rows[0], "item") != "pen" || pf16Cell(t, def, rows[0], "qty") != 1.0 {
		t.Fatalf("rejected row patch changed saved cells: %+v", rows)
	}
	err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pencil", "qty": 2, "unit-price": nil}),
	}})
	if err != nil {
		t.Fatalf("complete row with explicit null: %v", err)
	}
	rows = pf16Slips(t, e, id)
	if len(rows) != 1 || pf16Cell(t, def, rows[0], "item") != "pencil" || pf16Cell(t, def, rows[0], "qty") != 2.0 || pf16Cell(t, def, rows[0], "unit-price") != nil {
		t.Fatalf("complete row/null clear not applied: %+v", rows)
	}
}

func TestApproverFullTicketPatchReplacesDetailCollection(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: "second-slip-section", Code: "second-lines", Title: "Second lines", SectionType: model.SectionTypeSlip,
			SlipFields: []model.SlipField{pf16SlipField("second-item", "text")},
		})
	})
	initial := []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
		pf16Row("second-lines", map[string]any{"second-item": "notebook"}),
	}
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), initial)
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	// The full ticket endpoint replaces the complete detail collection. Omitting
	// a populated section would delete its row, which approvers are not allowed to do.
	err = pf16Update(e, approver, id, UpdateInput{SlipItems: &[]SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pencil", "qty": 2, "unit-price": 12}),
	}})
	if api, ok := kferr.As(err); !ok || api.Code != "validation_failed" {
		t.Fatalf("omitting populated approver detail section error=%v, want validation_failed", err)
	}
	rows := pf16Slips(t, e, id)
	if len(rows) != 2 {
		t.Fatalf("rejected replacement changed detail rows: %+v", rows)
	}
	complete := []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pencil", "qty": 2, "unit-price": 12}),
		pf16Row("second-lines", map[string]any{"second-item": "notebook"}),
	}
	if err = pf16Update(e, approver, id, UpdateInput{SlipItems: &complete}); err != nil {
		t.Fatalf("complete approver detail replacement rejected: %v", err)
	}
	rows = pf16Slips(t, e, id)
	if len(rows) != 2 {
		t.Fatalf("replacement detail rows=%d, want 2", len(rows))
	}
	def := pf16Def(t, e, "expense-claim")
	if got := pf16Cell(t, def, rows[0], "item"); got != "pencil" {
		t.Fatalf("replacement first section item=%v, want pencil", got)
	}
}

func TestCalculationRecalculationUsesActorInputPermissions(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				field := &def.SectionList[i].FormFields[j]
				switch field.Code {
				case "purpose":
					field.Approver = true
				case "total":
					field.Formula = pf16Formula("$purpose")
					field.Author, field.Approver = true, false
				case "approver-total":
					field.Formula = pf16Formula("$purpose")
					field.Author, field.Approver = false, true
				}
			}
			switch def.SectionList[i].SectionType {
			case model.SectionTypeForm:
				f := pf16FormField("approver-total", "calculation")
				f.Formula = pf16Formula("$purpose")
				f.Author, f.Approver = false, true
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, f)
			case model.SectionTypeSlip:
				authorCalc := pf16SlipField("author-total", "calculation")
				authorCalc.Formula = pf16Formula("$qty")
				authorCalc.Author = true
				approverCalc := pf16SlipField("approver-slip-total", "calculation")
				approverCalc.Formula = pf16Formula("$qty")
				approverCalc.Author, approverCalc.Approver = false, true
				def.SectionList[i].SlipFields = append(def.SectionList[i].SlipFields, authorCalc, approverCalc)
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "before"}), []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10})})
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != "before" {
		t.Fatalf("applicant calculation initial value=%v, want before", got)
	}
	if got := pf16FormValue(t, e, id, "approver-total"); got != nil {
		t.Fatalf("approver-only calculation ran for applicant: %v", got)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if got := pf16Cell(t, def, rows[0], "author-total"); got != 1.0 {
		t.Fatalf("applicant detail calculation initial value=%v, want 1", got)
	}
	if got := pf16Cell(t, def, rows[0], "approver-slip-total"); got != nil {
		t.Fatalf("approver-only detail calculation ran for applicant: %v", got)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	if err = pf16UpdateForm(e, approver, id, FormPatch{
		Inputs:    []FieldInput{pf14Input("purpose", "", "after")},
		SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 2, "unit-price": 10})},
	}); err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != "before" {
		t.Fatalf("applicant-only calculation changed during approver edit: %v", got)
	}
	if got := pf16FormValue(t, e, id, "approver-total"); got != "after" {
		t.Fatalf("approver calculation value=%v, want after", got)
	}
	rows = pf16Slips(t, e, id)
	if got := pf16Cell(t, def, rows[0], "author-total"); got != 1.0 {
		t.Fatalf("applicant-only detail calculation changed during approver edit: %v", got)
	}
	if got := pf16Cell(t, def, rows[0], "approver-slip-total"); got != 2.0 {
		t.Fatalf("approver detail calculation value=%v, want 2", got)
	}
}

func TestFormVariablesTakePrecedenceOverMetadata(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			if def.SectionList[i].SectionType != model.SectionTypeForm {
				continue
			}
			author := pf16FormField("author", "text")
			author.Author = true
			def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, author)
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "total" {
					def.SectionList[i].FormFields[j].Formula = pf16Formula("$author")
				}
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "travel", "author": "form-value"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != "form-value" {
		t.Fatalf("form variable collision resolved to %v, want form-value", got)
	}
}

func TestTicketTitleInputModeIsEnforced(t *testing.T) {
	for _, mode := range []string{"none", "calculate"} {
		t.Run(mode, func(t *testing.T) {
			e := pf14Setup(t)
			pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
				def.TitleInputMode = mode
				if mode == "calculate" {
					def.TitleFormula = pf16Formula(`CONCAT("calc:", purpose)`)
				}
			})
			workflow, _ := pf14FormDefinition(t, e, "expense-claim")
			inputs := pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "travel"})
			provided := "untrusted title"
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Title: &provided, Inputs: &inputs})
				return err
			})
			if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
				t.Fatalf("supplied title error=%v, want invalid_parameter", err)
			}
		})
	}
	eNone := pf14Setup(t)
	pf16Redefine(t, eNone, "expense-claim", func(def *model.WorkflowDef) { def.TitleInputMode = "none" })
	noneID, err := pf16Create(eNone, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, eNone, map[string]any{"amount": "100", "purpose": "travel"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if title := eNone.ticket(noneID).Title; title != nil {
		t.Fatalf("title in none mode=%v, want null", title)
	}
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		def.TitleInputMode = "calculate"
		def.TitleFormula = pf16Formula(`CONCAT("calc:", purpose)`)
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "travel"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	provided := "manual overwrite"
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Title: &provided}); err == nil {
		t.Fatal("calculated title was overwritten by title-only update")
	}
	if title := e.ticket(id).Title; title == nil || *title != "calc:travel" {
		t.Fatalf("calculated title=%v", title)
	}
}

func TestApproverUpdateIgnoresNonFormParameters(t *testing.T) {
	e := pf14Setup(t)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), nil)
	if err != nil {
		t.Fatal(err)
	}
	def := pf16Def(t, e, "expense-claim")
	var purposeID string
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			if field.Code == "purpose" {
				purposeID = field.ID
			}
		}
	}
	if purposeID == "" {
		t.Fatal("purpose field not found")
	}
	// If the ignored status were used for form validation, this missing
	// required applicant field would make the approver's unrelated edit fail.
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "purpose" {
					def.SectionList[i].FormFields[j].Required = true
				}
			}
		}
	})
	if _, err = e.db.Q().ExecContext(e.ctx, `DELETE FROM ticket_inputs WHERE ticket_id=? AND form_field_id=?`, id, purposeID); err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	status := model.TicketStatusInProgress
	title := "ignored"
	if err = pf16Update(e, approver, id, UpdateInput{Status: &status, Title: &title, Inputs: inputPatch("approver-note", "edited")}); err != nil {
		t.Fatalf("approver form update with ignored status/title: %v", err)
	}
	if got := e.ticket(id).Status; got != model.TicketStatusInProgress {
		t.Fatalf("ticket status=%s, want in_progress", got)
	}
	if got := pf16FormValue(t, e, id, "approver-note"); got != "edited" {
		t.Fatalf("approver input=%v, want edited", got)
	}
}

func inputPatch(code string, value any) *[]FieldInput {
	inputs := []FieldInput{pf14Input(code, "", value)}
	return &inputs
}
