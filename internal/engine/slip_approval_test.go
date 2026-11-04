package engine

import (
	"testing"

	"punchflow/internal/model"
)

func TestApproverRequiredInputUsesCurrentRouteStepCode(t *testing.T) {
	setup := func(t *testing.T, requiredAt string) (*pf14TestEnv, string) {
		e := pf14Setup(t)
		compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"))
		pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
			def.Overwritable = false
			section := &def.SectionList[0]
			section.FormFields = append(section.FormFields, model.FormField{ID: "approver-input-id", Code: "approval-note", FieldType: "text", Approver: true, ApproverEditRestriction: &model.ApproverEditRestriction{RouteStepCodes: []string{requiredAt}}})
		})
		id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), nil)
		if err != nil {
			t.Fatal(err)
		}
		return e, id
	}
	t.Run("only matching step requires input", func(t *testing.T) {
		e, id := setup(t, "review")
		_, err := pf15Approve(e, "sales-manager", id)
		pf15WantCode(t, "approve at configured step", err, "input_required_before_approve")
	})
	t.Run("unconfigured step is not blocked", func(t *testing.T) {
		e, id := setup(t, "accounting")
		compatApprove(t, e, "sales-manager", id)
		if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
			t.Fatalf("status=%s, want completed", got)
		}
	})
}

// PF-15/item 3: approver-required detail cells block approval like form fields.
func TestApproveRequiresApproverDetailCells(t *testing.T) {
	setup := func(t *testing.T, mutate func(*model.SlipField)) (*pf14TestEnv, string) {
		e := pf14Setup(t)
		compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"))
		pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
			def.Overwritable = false
			field := pf16SlipField("reviewer-note", "text")
			field.Approver, field.Author, field.Required = true, false, true
			if mutate != nil {
				mutate(&field)
			}
			lines := pf16Section(def, "lines")
			lines.SlipFields = append(lines.SlipFields, field)
		})
		id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{
			pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
		})
		if err != nil {
			t.Fatal(err)
		}
		return e, id
	}

	t.Run("blocked until filled", func(t *testing.T) {
		e, id := setup(t, nil)
		_, err := pf15Approve(e, "sales-manager", id)
		pf15WantCode(t, "approve with empty required cell", err, "input_required_before_approve")
		if got := e.ticket(id).Status; got != model.TicketStatusInProgress || e.ticket(id).CurrentStep != 1 {
			t.Fatalf("refused approval moved the ticket: %s step %d", got, e.ticket(id).CurrentStep)
		}
		if err = pf16UpdateForm(e, e.actor("sales-manager"), id, FormPatch{SlipItems: []SlipItemInput{
			pf16Row("lines", map[string]any{"reviewer-note": "checked"}),
		}}); err != nil {
			t.Fatalf("approver fills the cell: %v", err)
		}
		compatApprove(t, e, "sales-manager", id)
		if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
			t.Fatalf("status=%s, want completed", got)
		}
	})
	t.Run("every row must be filled", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"))
		pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
			def.Overwritable = false
			field := pf16SlipField("reviewer-note", "text")
			field.Approver, field.Author, field.Required = true, false, true
			lines := pf16Section(def, "lines")
			lines.SlipFields = append(lines.SlipFields, field)
		})
		id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base), []SlipItemInput{
			pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 10}),
			pf16Row("lines", map[string]any{"item": "ink", "qty": 2, "unit-price": 20}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err = pf16UpdateForm(e, e.actor("sales-manager"), id, FormPatch{SlipItems: []SlipItemInput{
			pf16Row("lines", map[string]any{"reviewer-note": "first only"}),
			pf16Row("lines", map[string]any{"reviewer-note": ""}),
		}}); err != nil {
			t.Fatal(err)
		}
		_, err = pf15Approve(e, "sales-manager", id)
		pf15WantCode(t, "approve with one empty row", err, "input_required_before_approve")
	})
	t.Run("optional cells do not block", func(t *testing.T) {
		e, id := setup(t, func(f *model.SlipField) { f.Required = false })
		compatApprove(t, e, "sales-manager", id)
		if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
			t.Fatalf("status=%s, want completed", got)
		}
	})
	t.Run("hidden cells cannot be required", func(t *testing.T) {
		hidden := true
		e, id := setup(t, func(f *model.SlipField) { f.Hidden = &hidden })
		compatApprove(t, e, "sales-manager", id)
		if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
			t.Fatalf("status=%s, want completed", got)
		}
	})
}
