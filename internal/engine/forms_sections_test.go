package engine

import (
	"fmt"
	"testing"

	"punchflow/internal/access"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

const pf16ConfSection = "00000000-0000-4000-8000-0000000000c1"

// pf16RestrictApproverNote moves approver-note into a restricted section that
// the author may not view; approvers and the ceo (section viewer) may.
func pf16RestrictApproverNote(t *testing.T, e *pf14TestEnv) {
	t.Helper()
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		var note model.FormField
		for i := range def.SectionList {
			kept := def.SectionList[i].FormFields[:0:0]
			for _, f := range def.SectionList[i].FormFields {
				if f.Code == "approver-note" {
					note = f
					continue
				}
				kept = append(kept, f)
			}
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = kept
			}
		}
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: pf16ConfSection, Title: "Confidential", SectionType: model.SectionTypeForm, FormFields: []model.FormField{note},
			ViewRestriction: true, AllowViewAuthor: false, AllowViewApprover: true,
			SectionViewers: []model.SectionViewer{{ID: "00000000-0000-4000-8000-0000000000c2", UserCode: "ceo"}},
		})
	})
}

func pf16NoteValue(t *testing.T, detail present.TicketDetailJSON, caller *access.Caller) (viewable bool, inputs int, topLevel bool) {
	t.Helper()
	for _, raw := range detail["ticketSections"].([]any) {
		section := raw.(map[string]any)
		if section["sectionId"] == pf16ConfSection {
			viewable = section["viewable"].(bool)
			inputs = len(section["inputs"].([]any))
			if section["viewRestriction"] != true || section["allowViewAuthor"] != false || section["allowViewApprover"] != true {
				t.Errorf("section flags=%v", section)
			}
			if viewers := section["sectionViewers"].([]any); len(viewers) != 1 {
				t.Errorf("sectionViewers=%v want the ceo", viewers)
			}
		}
	}
	for _, raw := range detail["inputs"].([]any) {
		field := raw.(map[string]any)["formField"].(map[string]any)
		if field["code"] == "approver-note" {
			topLevel = true
		}
	}
	return
}

func TestSectionViewRestrictions(t *testing.T) {
	e := pf14Setup(t)
	pf16RestrictApproverNote(t, e)
	skip := []string{"approver-note"}
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, pf16Base, skip...), nil)
	if err != nil {
		t.Fatalf("author create without restricted section: %v", err)
	}
	// The author may not enter the restricted section.
	_, err = pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
		t.Fatalf("author supplying restricted field error=%v, want invalid_parameter", err)
	}
	approver := Actor{UserID: pf14cCurrentApprover(t, e, id)}
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	if err = pf16UpdateForm(e, approver, id, FormPatch{Inputs: []FieldInput{pf14Input("approver-note", fields["approver-note"], "secret")}}); err != nil {
		t.Fatalf("approver entering restricted section: %v", err)
	}

	detailFor := func(c *access.Caller) present.TicketDetailJSON {
		d, err := present.TicketDetailFor(e.ctx, e.db.Q(), id, c)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	for name, tc := range map[string]struct {
		caller   *access.Caller
		viewable bool
	}{
		"author":         {&access.Caller{UserID: e.user("sales-rep")}, false},
		"stranger":       {&access.Caller{UserID: e.user("hr-manager")}, false},
		"approver":       {&access.Caller{UserID: approver.UserID}, true},
		"section viewer": {&access.Caller{UserID: e.user("ceo")}, true},
		"ticket_read":    {&access.Caller{UserID: e.user("hr-manager"), TicketRead: true}, true},
		"internal (nil)": {nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			viewable, inputs, topLevel := pf16NoteValue(t, detailFor(tc.caller), tc.caller)
			wantInputs := 0
			if tc.viewable {
				wantInputs = 1
			}
			if viewable != tc.viewable || inputs != wantInputs || topLevel != tc.viewable {
				t.Fatalf("viewable=%v inputs=%d topLevel=%v, want viewable=%v inputs=%d", viewable, inputs, topLevel, tc.viewable, wantInputs)
			}
		})
	}
}

func TestRestrictedSectionValuesSurviveAuthorEdits(t *testing.T) {
	e := pf14Setup(t)
	pf16RestrictApproverNote(t, e)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base, "approver-note"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES('seed-note',?,?,'"secret"','[]',0,0)`, id, fields["approver-note"])
		return err
	})
	check := func(step string) {
		t.Helper()
		for _, row := range mustTicketInputs(t, e, id) {
			if row.FormFieldID == fields["approver-note"] && fmt.Sprint(row.Value) == "secret" {
				return
			}
		}
		t.Fatalf("%s dropped the restricted value", step)
	}
	if err = pf16Update(e, e.actor("sales-rep"), id, UpdateInput{Inputs: ptr(pf16Inputs(t, e, map[string]any{"amount": "5", "purpose": "again"}, "approver-note"))}); err != nil {
		t.Fatal(err)
	}
	check("full applicant update")
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("purpose", fields["purpose"], "third")}}); err != nil {
		t.Fatal(err)
	}
	check("partial form update")
}

func TestSectionAccessIncludesProxyApplicantsAndActiveProxyApprovers(t *testing.T) {
	e := pf14Setup(t)
	pf16RestrictApproverNote(t, e)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			if def.SectionList[i].ID == pf16ConfSection {
				def.SectionList[i].AllowViewAuthor = true
			}
		}
	})
	inputs := pf16Inputs(t, e, pf16Base)
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	approverID := pf14cCurrentApprover(t, e, id)
	proxyID := e.user("hr-manager")
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	workflowIDs := fmt.Sprintf(`["%s"]`, workflow.ID)
	if _, err = e.db.Q().ExecContext(e.ctx, `INSERT INTO proxy_approvers(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES('active-proxy',?,?,NULL,NULL,?,?,?)`, approverID, proxyID, workflowIDs, e.clock.Now().UnixMilli(), e.clock.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET proxy_client_user_id=? WHERE id=?`, proxyID, id); err != nil {
		t.Fatal(err)
	}
	detailFor := func(userID string) present.TicketDetailJSON {
		t.Helper()
		detail, err := present.TicketDetailFor(e.ctx, e.db.Q(), id, &access.Caller{UserID: userID})
		if err != nil {
			t.Fatal(err)
		}
		return detail
	}
	if !pf16SectionViewable(detailFor(proxyID)) {
		t.Fatal("proxy applicant cannot view author-enabled restricted section")
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET proxy_client_user_id=NULL WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = pf16UpdateForm(e, Actor{UserID: proxyID}, id, FormPatch{Inputs: []FieldInput{pf14Input("approver-note", fields["approver-note"], "proxy value")}}); err != nil {
		t.Fatalf("active proxy approver cannot update restricted field: %v", err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE proxy_approvers SET ends_on='2025-02-02' WHERE id='active-proxy'`); err != nil {
		t.Fatal(err)
	}
	if pf16SectionViewable(detailFor(proxyID)) {
		t.Fatal("expired proxy approver can view restricted section")
	}
	if err = pf16UpdateForm(e, Actor{UserID: proxyID}, id, FormPatch{Inputs: []FieldInput{pf14Input("approver-note", fields["approver-note"], "expired proxy")}}); err == nil {
		t.Fatal("expired proxy approver updated restricted field")
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET proxy_client_user_id=NULL,status='rejected' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if pf16SectionViewable(detailFor(approverID)) {
		t.Fatal("former approver can view restricted section on rejected ticket")
	}
	if pf16SectionViewable(detailFor(proxyID)) {
		t.Fatal("proxy approver can view restricted section on rejected ticket")
	}
	if err = pf16UpdateForm(e, Actor{UserID: proxyID}, id, FormPatch{Inputs: []FieldInput{pf14Input("approver-note", fields["approver-note"], "not allowed")}}); err == nil {
		t.Fatal("proxy approver updated restricted field after ticket left in-progress state")
	}
}

func pf16SectionViewable(detail present.TicketDetailJSON) bool {
	for _, raw := range detail["ticketSections"].([]any) {
		section := raw.(map[string]any)
		if section["sectionId"] == pf16ConfSection {
			return section["viewable"].(bool)
		}
	}
	return false
}
