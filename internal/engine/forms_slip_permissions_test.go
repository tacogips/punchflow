package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

func TestApproverDetailRowAdditionPermission(t *testing.T) {
	for _, endpoint := range []string{"ticket", "form"} {
		t.Run(endpoint, func(t *testing.T) {
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
			if err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: []SlipItemInput{
				pf16Row("lines", map[string]any{"reviewer-note": "before"}),
			}}); err != nil {
				t.Fatal(err)
			}
			rows := []SlipItemInput{
				pf16Row("lines", map[string]any{"reviewer-note": "edited"}),
				pf16Row("lines", map[string]any{"reviewer-note": "new row"}),
			}
			if endpoint == "form" {
				err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: rows})
			} else {
				err = pf16Update(e, approver, id, UpdateInput{SlipItems: &rows})
			}
			if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" || api.Status != 400 {
				t.Fatalf("adding a row with permission disabled: error=%v, want 400 invalid_parameter", err)
			}
			// Existing row cells remain editable when row addition is disabled.
			onlyExisting := []SlipItemInput{pf16Row("lines", map[string]any{"reviewer-note": "edited"})}
			if endpoint == "form" {
				err = pf16UpdateForm(e, approver, id, FormPatch{SlipItems: onlyExisting})
			} else {
				err = pf16Update(e, approver, id, UpdateInput{SlipItems: &onlyExisting})
			}
			if err != nil {
				t.Fatalf("editing existing row cell: %v", err)
			}
			def := pf16Def(t, e, "expense-claim")
			stored := pf16Slips(t, e, id)
			if len(stored) != 1 || pf16Cell(t, def, stored[0], "reviewer-note") != "edited" {
				t.Fatalf("stored rows=%+v; want existing row edited and no extra rows", stored)
			}
		})
	}
}

func TestApproverMayAddDetailRowsWhenEnabled(t *testing.T) {
	e := pf14Setup(t)
	pf16TwoSections(t, e)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		def.Overwritable = false
		for i := range def.SectionList {
			if def.SectionList[i].Code == "lines" {
				def.SectionList[i].AllowApproverToAddItems = true
			}
		}
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
	})
	if err != nil {
		t.Fatal(err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	rows := []SlipItemInput{
		pf16Row("lines", map[string]any{"reviewer-note": "existing"}),
		pf16Row("lines", map[string]any{"reviewer-note": "added"}),
	}
	if err := pf16UpdateForm(e, approver, id, FormPatch{SlipItems: rows}); err != nil {
		t.Fatalf("adding row when enabled: %v", err)
	}
	if got := len(pf16Slips(t, e, id)); got != 2 {
		t.Fatalf("stored rows=%d, want 2", got)
	}
}
