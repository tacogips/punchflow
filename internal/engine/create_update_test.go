package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14cCreate(t *testing.T, e *pf14TestEnv, workflowCode, actorCode string, values map[string]any, proxyClientID *string) (string, error) {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), workflowCode)
	if err != nil {
		return "", err
	}
	_, fields := pf14FormDefinition(t, e, workflowCode)
	inputs := pf14CompleteInputs(fields, values)
	teamID := e.team("sales")
	if actorCode == "assistant" {
		if err = e.db.Q().QueryRowContext(e.ctx, `SELECT m.team_id FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE m.user_id=? AND c.current=1 ORDER BY m.team_id LIMIT 1`, e.user(actorCode)).Scan(&teamID); err != nil {
			return "", err
		}
	}
	var ticketID string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var createErr error
		ticketID, _, createErr = e.engine.Create(e.ctx, tx, e.actor(actorCode), CreateInput{
			Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: teamID,
			Inputs: &inputs, ProxyClientUserID: proxyClientID,
		})
		return createErr
	})
	return ticketID, err
}

func pf14cMasterItemID(t *testing.T, e *pf14TestEnv, masterCode, itemCode string) string {
	t.Helper()
	var id string
	err := e.db.Q().QueryRowContext(e.ctx, `SELECT i.id FROM general_master_items i JOIN general_masters m ON m.id=i.general_master_id WHERE m.code=? AND i.code=?`, masterCode, itemCode).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func pf14cPurchaseValues(e *pf14TestEnv) map[string]any {
	return map[string]any{
		"amount": "100", "item": "Laptop", "urgent": []string{"yes"},
		"reviewer": e.user("engineer"), "approving-team": e.team("engineering"),
		"vendor": pf14cMasterItemID(e.t, e, "vendors", "vendor-acme"), "budget-code": "IT-2025",
	}
}

func TestFormFieldTypes(t *testing.T) {
	e := pf14Setup(t)
	validValues := pf14cPurchaseValues(e)
	var inactiveTeam string
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT id FROM teams WHERE organization_chart_id<>(SELECT id FROM organization_charts WHERE current=1) LIMIT 1`).Scan(&inactiveTeam); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	for k, v := range validValues {
		values[k] = v
	}
	cases := []struct {
		name, field string
		value       any
		wantErr     bool
	}{
		{"unknown user", "reviewer", "missing-user-id", true},
		{"existing user", "reviewer", e.user("engineer"), false},
		{"inactive chart team", "approving-team", inactiveTeam, true},
		{"active chart team", "approving-team", e.team("engineering"), false},
		{"unknown ticket", "related", "missing-ticket-id", true},
		{"wrong master", "vendor", pf14cMasterItemID(t, e, "cost-centers", "cc-eng"), true},
		{"right master", "vendor", pf14cMasterItemID(t, e, "vendors", "vendor-acme"), false},
		{"invalid checkbox option", "urgent", []string{"no"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := make(map[string]any, len(values)+1)
			for k, v := range values {
				input[k] = v
			}
			input[tc.field] = tc.value
			_, err := pf14cCreate(t, e, "purchase-request", "sales-rep", input, nil)
			if tc.wantErr {
				assertPF14FieldError(t, err, "validation_failed", tc.field)
			} else if err != nil {
				t.Fatalf("valid %s rejected: %v", tc.field, err)
			}
		})
	}
	ticketID := e.createExpense("sales-rep", 25, false)
	for _, tc := range []struct {
		name  string
		value any
		bad   bool
	}{{"existing ticket", ticketID, false}, {"invalid date", "2025/02/03", true}, {"valid date", "2025-02-03", false}} {
		t.Run(tc.name, func(t *testing.T) {
			expense := map[string]any{"amount": "100", "purpose": "travel", "related": tc.value, "expense-date": tc.value}
			if tc.name == "existing ticket" {
				expense["expense-date"] = "2025-02-03"
			} else {
				expense["related"] = nil
			}
			_, err := pf14cCreate(t, e, "expense-claim", "sales-rep", expense, nil)
			if tc.bad {
				assertPF14FieldError(t, err, "validation_failed", "expense-date")
			} else if err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
		})
	}
	t.Run("unknown signed file", func(t *testing.T) {
		_, err := pf14cCreate(t, e, "expense-claim", "sales-rep", map[string]any{"amount": "100", "purpose": "travel", "receipt": "missing-signed-id"}, nil)
		assertPF14FieldError(t, err, "validation_failed", "receipt")
	})
	leave, _ := store.GetWorkflowByCode(e.ctx, e.db.Q(), "leave-request")
	_, leaveFields := pf14FormDefinition(t, e, "leave-request")
	for _, tc := range []struct {
		name, reason string
		wantErr      bool
	}{{"invalid pull down", "other-value", true}, {"valid pull down", "vacation", false}} {
		t.Run(tc.name, func(t *testing.T) {
			inputs := pf14CompleteInputs(leaveFields, map[string]any{"start-date": "2025-02-04", "reason": tc.reason, "detail": "leave"})
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, _, err := e.engine.Create(e.ctx, tx, e.actor("hr-manager"), CreateInput{Status: model.TicketStatusDraft, WorkflowID: leave.ID, AuthorTeamID: e.team("hr"), Inputs: &inputs})
				return err
			})
			if tc.wantErr {
				assertPF14FieldError(t, err, "validation_failed", "reason")
			} else if err != nil {
				t.Fatalf("valid reason rejected: %v", err)
			}
		})
	}

	for _, tc := range []struct {
		field model.FormField
		value any
		bad   bool
	}{
		{field: model.FormField{FieldType: "number"}, value: "not-a-number", bad: true},
		{field: model.FormField{FieldType: "number"}, value: "NaN", bad: true},
		{field: model.FormField{FieldType: "integer"}, value: "1.5", bad: true},
		{field: model.FormField{FieldType: "datetime"}, value: "2025-02-03 09:30"},
		{field: model.FormField{FieldType: "datetime"}, value: "2025-02-03", bad: true},
		{field: model.FormField{FieldType: "radio_button", Options: []string{"a"}}, value: "b", bad: true},
	} {
		if err := pf14cValidateFormValue(e.ctx, e.db.Q(), tc.field, tc.value); (err != nil) != tc.bad {
			t.Errorf("field type %s value %v error=%v, want error %v", tc.field.FieldType, tc.value, err, tc.bad)
		}
	}
	minCalculationValue := 1.0
	calc := model.FormField{ID: "calculation-id", Code: "calculation", FieldType: "calculation", MinValue: &minCalculationValue}
	inputs := []FieldInput{pf14Input("calculation", calc.ID, "ignored")}
	def := model.WorkflowDef{SectionList: []model.WorkflowSection{{SectionType: model.SectionTypeForm, FormFields: []model.FormField{calc}}}}
	res, err := pf14NormalizeForm(e.ctx, e.db.Q(), "ticket-id", def, inputs, nil, formOpts{}, 0)
	rows := res.Inputs
	if err != nil || len(rows) != 1 || rows[0].Value != nil {
		t.Fatalf("calculation should normalize to null: rows=%+v err=%v", rows, err)
	}
}

func TestProxyApplicantCreate(t *testing.T) {
	t.Run("empty workflow restriction is unrestricted", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		if _, err := e.db.Q().ExecContext(e.ctx, `UPDATE proxy_applicants SET workflow_ids='[]' WHERE user_id=? AND proxy_user_id=?`, client, e.user("assistant")); err != nil {
			t.Fatal(err)
		}
		id, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "100", "purpose": "unrestricted proxy"}, &client)
		if err != nil {
			t.Fatalf("unrestricted proxy create: %v", err)
		}
		if got := e.ticket(id); got.ProxyClientUserID == nil || *got.ProxyClientUserID != client {
			t.Fatalf("proxy client=%v, want %s", got.ProxyClientUserID, client)
		}
	})
	t.Run("registered proxy creates with actor as author", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		id, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "250", "purpose": "travel"}, &client)
		if err != nil {
			t.Fatal(err)
		}
		ticket := e.ticket(id)
		if ticket.AuthorID == nil || *ticket.AuthorID != e.user("assistant") || ticket.ProxyClientUserID == nil || *ticket.ProxyClientUserID != client {
			t.Fatalf("ticket author/proxy client mismatch: %+v", ticket)
		}
	})
	t.Run("registration does not cover workflow", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		_, err := pf14cCreate(t, e, "purchase-request", "assistant", map[string]any{"amount": "100", "item": "Laptop", "budget-code": "IT"}, &client)
		assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
	})
	t.Run("expired registration is rejected", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		e.apply(func(tx *store.Tx) error {
			_, err := tx.ExecContext(e.ctx, `UPDATE proxy_applicants SET ends_on='2025-02-02' WHERE user_id=? AND proxy_user_id=?`, client, e.user("assistant"))
			return err
		})
		_, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "250", "purpose": "travel"}, &client)
		assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
	})
	t.Run("missing registration is rejected", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		_, err := pf14cCreate(t, e, "expense-claim", "sales-rep", map[string]any{"amount": "250", "purpose": "travel"}, &client)
		assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
	})
	t.Run("suspended client is rejected", func(t *testing.T) {
		e := pf14Setup(t)
		client := e.user("ceo")
		e.apply(func(tx *store.Tx) error {
			_, err := tx.ExecContext(e.ctx, `UPDATE users SET status='suspended' WHERE id=?`, client)
			return err
		})
		_, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "250", "purpose": "travel"}, &client)
		assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
	})
	t.Run("update rejects invalid client", func(t *testing.T) {
		e := pf14Setup(t)
		id := e.createExpense("sales-rep", 10, false)
		proxy := json.RawMessage(fmt.Sprintf("%q", e.user("ceo")))
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.Update(e.ctx, tx, e.actor("sales-rep"), id, UpdateInput{ProxyClientUserID: proxy})
			return err
		})
		assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
	})
	t.Run("update accepts registered proxy client", func(t *testing.T) {
		e := pf14Setup(t)
		id, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "250", "purpose": "travel"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		client := e.user("ceo")
		proxy := json.RawMessage(fmt.Sprintf("%q", client))
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.Update(e.ctx, tx, e.actor("assistant"), id, UpdateInput{ProxyClientUserID: proxy})
			return err
		})
		if err != nil {
			t.Fatalf("registered proxy update rejected: %v", err)
		}
		if got := e.ticket(id).ProxyClientUserID; got == nil || *got != client {
			t.Fatalf("proxy client=%v want %s", got, client)
		}
	})
	for _, status := range []string{"suspended", "deactivated"} {
		status := status
		t.Run("saved "+status+" client is revalidated and explicit null clears it", func(t *testing.T) {
			e := pf14Setup(t)
			client := e.user("ceo")
			id, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": "250", "purpose": "travel"}, &client)
			if err != nil {
				t.Fatal(err)
			}
			e.apply(func(tx *store.Tx) error {
				_, err := tx.ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, status, client)
				return err
			})
			err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, err := e.engine.Update(e.ctx, tx, e.actor("assistant"), id, UpdateInput{Title: compatStr("draft save")})
				return err
			})
			assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
			submit := model.TicketStatusInProgress
			err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, err := e.engine.Update(e.ctx, tx, e.actor("assistant"), id, UpdateInput{Status: &submit})
				return err
			})
			assertPF14FieldError(t, err, "validation_failed", "proxyClientUserId")
			clear := json.RawMessage("null")
			err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, err := e.engine.Update(e.ctx, tx, e.actor("assistant"), id, UpdateInput{ProxyClientUserID: clear})
				return err
			})
			if err != nil {
				t.Fatalf("explicit null should clear proxy client: %v", err)
			}
			if got := e.ticket(id).ProxyClientUserID; got != nil {
				t.Fatalf("proxy client after explicit null=%v, want nil", got)
			}
		})
	}
}

func TestUpdateFormMerge(t *testing.T) {
	e := pf14Setup(t)
	costCenter := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": "350", "purpose": "before", "cost-center": costCenter})
	costCenterRef, _ := json.Marshal(costCenter)
	for i := range inputs {
		if inputs[i].FormFieldID == fields["cost-center"] {
			inputs[i].Value = nil
			inputs[i].GeneralMasterItemID = costCenterRef
		}
	}
	var id string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var originalAmount, originalCost any
	var originalCostRefs []string
	for _, row := range mustTicketInputs(t, e, id) {
		switch row.FormFieldID {
		case fields["amount"]:
			originalAmount = row.Value
		case fields["cost-center"]:
			originalCost, originalCostRefs = row.Value, append([]string(nil), row.RefIDs...)
		}
	}
	if len(originalCostRefs) != 1 || originalCostRefs[0] != costCenter {
		t.Fatalf("initial cost-center references=%v want [%s]", originalCostRefs, costCenter)
	}
	patch := FormPatch{Inputs: []FieldInput{pf14Input("purpose", fields["purpose"], "after")}}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.UpdateForm(e.ctx, tx, e.actor("sales-rep"), id, patch)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := mustTicketInputs(t, e, id)
	seen := make(map[string]bool, 3)
	for _, row := range rows {
		switch row.FormFieldID {
		case fields["amount"]:
			seen["amount"] = true
			if fmt.Sprint(row.Value) != fmt.Sprint(originalAmount) {
				t.Errorf("amount changed: got %v want %v", row.Value, originalAmount)
			}
		case fields["purpose"]:
			seen["purpose"] = true
			if row.Value != "after" {
				t.Errorf("purpose=%v want after", row.Value)
			}
		case fields["cost-center"]:
			seen["cost-center"] = true
			if fmt.Sprint(row.Value) != fmt.Sprint(originalCost) || fmt.Sprint(row.RefIDs) != fmt.Sprint(originalCostRefs) {
				t.Errorf("cost center changed: value=%v refs=%v", row.Value, row.RefIDs)
			}
		}
	}
	for _, code := range []string{"amount", "purpose", "cost-center"} {
		if !seen[code] {
			t.Errorf("UpdateForm dropped %s row", code)
		}
	}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.UpdateForm(e.ctx, tx, e.actor("sales-rep"), id, FormPatch{})
		return err
	})
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
		t.Fatalf("empty patch error=%v, want invalid_parameter", err)
	}

	t.Run("applicant update missing field is invalid_parameter", func(t *testing.T) {
		e := pf14Setup(t)
		costCenter := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
		_, fields := pf14FormDefinition(t, e, "expense-claim")
		id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusDraft, map[string]any{
			"amount": "350", "purpose": "before", "cost-center": costCenter,
		}, nil)
		before := make(map[string]model.TicketInput, 3)
		for _, row := range mustTicketInputs(t, e, id) {
			row.RefIDs = append([]string(nil), row.RefIDs...)
			before[row.FormFieldID] = row
		}

		patch := []FieldInput{pf14Input("purpose", fields["purpose"], "x")}
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.Update(e.ctx, tx, e.actor("sales-rep"), id, UpdateInput{Inputs: &patch})
			return err
		})
		api, ok := kferr.As(err)
		if !ok || api.Code != "invalid_parameter" || api.Status != 400 {
			t.Fatalf("partial applicant update error=%v, want 400 invalid_parameter", err)
		}
		after := mustTicketInputs(t, e, id)
		if len(after) != len(before) {
			t.Fatalf("partial applicant update changed row count: got %d want %d", len(after), len(before))
		}
		for _, row := range after {
			old, exists := before[row.FormFieldID]
			if !exists {
				t.Errorf("partial applicant update added field %s", row.FormFieldID)
				continue
			}
			if fmt.Sprint(row.Value) != fmt.Sprint(old.Value) || fmt.Sprint(row.RefIDs) != fmt.Sprint(old.RefIDs) {
				t.Errorf("partial applicant update changed field %s: value=%v refs=%v; want value=%v refs=%v", row.FormFieldID, row.Value, row.RefIDs, old.Value, old.RefIDs)
			}
		}
	})

	t.Run("approver update keeps applicant inputs", func(t *testing.T) {
		e := pf14Setup(t)
		costCenter := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
		_, fields := pf14FormDefinition(t, e, "expense-claim")
		id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{
			"amount": "350", "purpose": "before", "cost-center": costCenter,
		}, nil)
		ticket := e.ticket(id)
		steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
		if err != nil {
			t.Fatal(err)
		}
		approverID := ""
		for _, step := range steps {
			if step.StepOrder != ticket.CurrentStep || step.Status != model.StepStatusInProgress {
				continue
			}
			assignees, er := store.EngStepAssignees(e.ctx, e.db.Q(), step.ID)
			if er != nil {
				t.Fatal(er)
			}
			for _, assignee := range assignees {
				if !assignee.Completed {
					approverID = assignee.UserID
					break
				}
			}
			if approverID != "" {
				break
			}
		}
		if approverID == "" {
			t.Fatal("current step has no unfinished assignee")
		}
		before := make(map[string]model.TicketInput, 3)
		for _, row := range mustTicketInputs(t, e, id) {
			row.RefIDs = append([]string(nil), row.RefIDs...)
			before[row.FormFieldID] = row
		}
		patch := []FieldInput{pf14Input("approver-note", fields["approver-note"], "ok")}
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.Update(e.ctx, tx, Actor{UserID: approverID}, id, UpdateInput{Inputs: &patch})
			return err
		})
		if err != nil {
			t.Fatalf("approver partial update: %v", err)
		}
		seen := map[string]bool{}
		for _, row := range mustTicketInputs(t, e, id) {
			code := ""
			for _, candidate := range []string{"amount", "purpose", "cost-center", "approver-note"} {
				if fields[candidate] == row.FormFieldID {
					code = candidate
					break
				}
			}
			if code == "" {
				continue
			}
			seen[code] = true
			if code == "approver-note" {
				if row.Value != "ok" {
					t.Errorf("approver-note=%v want ok", row.Value)
				}
				continue
			}
			old := before[row.FormFieldID]
			if fmt.Sprint(row.Value) != fmt.Sprint(old.Value) || fmt.Sprint(row.RefIDs) != fmt.Sprint(old.RefIDs) {
				t.Errorf("approver update changed %s: value=%v refs=%v; want value=%v refs=%v", code, row.Value, row.RefIDs, old.Value, old.RefIDs)
			}
		}
		for _, code := range []string{"amount", "purpose", "cost-center", "approver-note"} {
			if !seen[code] {
				t.Errorf("approver update missing %s row", code)
			}
		}
	})

	t.Run("approver field rule honors overwritable", func(t *testing.T) {
		for _, path := range []string{"Update", "UpdateForm"} {
			t.Run(path+" rejects applicant field when not overwritable", func(t *testing.T) {
				e := pf14Setup(t)
				pf14cSetOverwritable(t, e, false)
				costCenter := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
				_, fields := pf14FormDefinition(t, e, "expense-claim")
				id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{
					"amount": "350", "purpose": "before", "cost-center": costCenter,
				}, nil)
				approverID := pf14cCurrentApprover(t, e, id)
				before := pf14cInputSnapshot(t, e, id)
				patch := []FieldInput{pf14Input("amount", fields["amount"], "999")}
				err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
					var err error
					if path == "Update" {
						_, err = e.engine.Update(e.ctx, tx, Actor{UserID: approverID}, id, UpdateInput{Inputs: &patch})
					} else {
						_, err = e.engine.UpdateForm(e.ctx, tx, Actor{UserID: approverID}, id, FormPatch{Inputs: patch})
					}
					return err
				})
				api, ok := kferr.As(err)
				if !ok || api.Code != "invalid_parameter" || api.Status != 400 {
					t.Fatalf("%s error=%v, want 400 invalid_parameter", path, err)
				}
				pf14cAssertInputSnapshot(t, e, id, before)
			})
		}

		t.Run("approver field accepted when overwritable", func(t *testing.T) {
			e := pf14Setup(t)
			pf14cSetOverwritable(t, e, true)
			_, fields := pf14FormDefinition(t, e, "expense-claim")
			id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{
				"amount": "350", "purpose": "before", "cost-center": pf14cMasterItemID(t, e, "cost-centers", "cc-eng"),
			}, nil)
			approverID := pf14cCurrentApprover(t, e, id)
			patch := []FieldInput{pf14Input("amount", fields["amount"], "999")}
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, err := e.engine.Update(e.ctx, tx, Actor{UserID: approverID}, id, UpdateInput{Inputs: &patch})
				return err
			})
			if err != nil {
				t.Fatalf("overwritable approver amount update rejected: %v", err)
			}
			if got := pf14cInputSnapshot(t, e, id)[fields["amount"]].Value; fmt.Sprint(got) != "999" {
				t.Fatalf("amount=%v want 999", got)
			}
		})

		t.Run("approver note accepted when not overwritable", func(t *testing.T) {
			e := pf14Setup(t)
			pf14cSetOverwritable(t, e, false)
			costCenter := pf14cMasterItemID(t, e, "cost-centers", "cc-eng")
			_, fields := pf14FormDefinition(t, e, "expense-claim")
			id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{
				"amount": "350", "purpose": "before", "cost-center": costCenter,
			}, nil)
			before := pf14cInputSnapshot(t, e, id)
			approverID := pf14cCurrentApprover(t, e, id)
			patch := []FieldInput{pf14Input("approver-note", fields["approver-note"], "ok")}
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				_, err := e.engine.UpdateForm(e.ctx, tx, Actor{UserID: approverID}, id, FormPatch{Inputs: patch})
				return err
			})
			if err != nil {
				t.Fatalf("approver-note patch rejected: %v", err)
			}
			after := pf14cInputSnapshot(t, e, id)
			for fieldID, old := range before {
				if fieldID == fields["approver-note"] {
					continue
				}
				got := after[fieldID]
				if fmt.Sprint(got.Value) != fmt.Sprint(old.Value) || fmt.Sprint(got.RefIDs) != fmt.Sprint(old.RefIDs) {
					t.Errorf("approver-note patch changed field %s: got value=%v refs=%v, want value=%v refs=%v", fieldID, got.Value, got.RefIDs, old.Value, old.RefIDs)
				}
			}
			if got := after[fields["approver-note"]].Value; got != "ok" {
				t.Errorf("approver-note=%v want ok", got)
			}
		})
	})

	for _, path := range []string{"Update", "UpdateForm"} {
		t.Run("author in progress cannot "+path, func(t *testing.T) {
			e := pf14Setup(t)
			_, fields := pf14FormDefinition(t, e, "expense-claim")
			id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{
				"amount": "350", "purpose": "before", "cost-center": pf14cMasterItemID(t, e, "cost-centers", "cc-eng"),
			}, nil)
			before := pf14cInputSnapshot(t, e, id)
			if pf14cCurrentApprover(t, e, id) == e.user("sales-rep") {
				t.Fatal("fixture author unexpectedly assigned as current approver")
			}
			complete := pf14CompleteInputs(fields, map[string]any{
				"amount": "999", "purpose": "changed", "cost-center": pf14cMasterItemID(t, e, "cost-centers", "cc-eng"),
			})
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				var err error
				if path == "Update" {
					_, err = e.engine.Update(e.ctx, tx, e.actor("sales-rep"), id, UpdateInput{Inputs: &complete})
				} else {
					patch := []FieldInput{pf14Input("purpose", fields["purpose"], "changed")}
					_, err = e.engine.UpdateForm(e.ctx, tx, e.actor("sales-rep"), id, FormPatch{Inputs: patch})
				}
				return err
			})
			api, ok := kferr.As(err)
			if !ok || api.Code != "user_not_assigned" || api.Status != 403 {
				t.Fatalf("author %s error=%v, want 403 user_not_assigned from current-approver check", path, err)
			}
			pf14cAssertInputSnapshot(t, e, id, before)
		})
	}
}

func pf14cSetOverwritable(t *testing.T, e *pf14TestEnv, overwritable bool) {
	t.Helper()
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE workflows SET definition_json=json_set(definition_json,'$.overwritable',json(?)) WHERE code='expense-claim'`, fmt.Sprint(overwritable))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pf14cCurrentApprover(t *testing.T, e *pf14TestEnv, id string) string {
	t.Helper()
	ticket := e.ticket(id)
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.StepOrder != ticket.CurrentStep || step.Status != model.StepStatusInProgress {
			continue
		}
		assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), step.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, assignee := range assignees {
			if !assignee.Completed {
				return assignee.UserID
			}
		}
	}
	t.Fatal("current step has no unfinished assignee")
	return ""
}

func pf14cInputSnapshot(t *testing.T, e *pf14TestEnv, id string) map[string]model.TicketInput {
	t.Helper()
	snapshot := make(map[string]model.TicketInput)
	for _, row := range mustTicketInputs(t, e, id) {
		row.RefIDs = append([]string(nil), row.RefIDs...)
		snapshot[row.FormFieldID] = row
	}
	return snapshot
}

func pf14cAssertInputSnapshot(t *testing.T, e *pf14TestEnv, id string, before map[string]model.TicketInput) {
	t.Helper()
	after := pf14cInputSnapshot(t, e, id)
	if len(after) != len(before) {
		t.Fatalf("ticket input row count changed: got %d want %d", len(after), len(before))
	}
	for fieldID, old := range before {
		got, exists := after[fieldID]
		if !exists {
			t.Errorf("ticket input %s was removed", fieldID)
			continue
		}
		if fmt.Sprint(got.Value) != fmt.Sprint(old.Value) || fmt.Sprint(got.RefIDs) != fmt.Sprint(old.RefIDs) {
			t.Errorf("ticket input %s changed: got value=%v refs=%v; want value=%v refs=%v", fieldID, got.Value, got.RefIDs, old.Value, old.RefIDs)
		}
	}
}

func mustTicketInputs(t *testing.T, e *pf14TestEnv, id string) []model.TicketInput {
	t.Helper()
	rows, err := store.EngTicketInputs(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
