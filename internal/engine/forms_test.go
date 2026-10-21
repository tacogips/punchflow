package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14FormDefinition(t *testing.T, e *pf14TestEnv, code string) (model.Workflow, map[string]string) {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), code)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, section := range definition.SectionList {
		for _, field := range section.FormFields {
			fields[field.Code] = field.ID
		}
	}
	return workflow, fields
}

func pf14Input(code, id string, value any) FieldInput {
	raw, _ := json.Marshal(value)
	return FieldInput{FormFieldCode: code, FormFieldID: id, Value: raw}
}

func pf14CompleteInputs(fields map[string]string, values map[string]any) []FieldInput {
	inputs := make([]FieldInput, 0, len(fields))
	for code, id := range fields {
		inputs = append(inputs, pf14Input(code, id, values[code]))
	}
	return inputs
}

func TestFormValidation(t *testing.T) {
	t.Run("unknown field code", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "expense-claim")
		inputs := []FieldInput{pf14Input("unknown", "", "value")}
		status := model.TicketStatusDraft
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
			return err
		})
		if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
			t.Fatalf("unknown field error=%v (known amount field=%s)", err, fields["amount"])
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "expense-claim")
		status := model.TicketStatusInProgress
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales")})
			return err
		})
		if api, ok := kferr.As(err); !ok || api.Code != "invalid_parameter" {
			t.Fatalf("missing fields error=%v (amount=%s purpose=%s)", err, fields["amount"], fields["purpose"])
		}
	})

	t.Run("draft skips required checks but submit enforces them", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "expense-claim")
		inputs := pf14CompleteInputs(fields, map[string]any{"amount": "", "purpose": ""})
		draft := model.TicketStatusDraft
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: draft, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
			return err
		})
		if err != nil {
			t.Fatalf("draft with empty required fields: %v", err)
		}
		progress := model.TicketStatusInProgress
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: progress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
			return err
		})
		if api, ok := kferr.As(err); !ok || api.Code != "validation_failed" {
			t.Fatalf("submit with empty required fields error=%v", err)
		}
	})

	t.Run("conditional details are required for other reason", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "leave-request")
		inputs := pf14CompleteInputs(fields, map[string]any{"start-date": "2025-02-10", "reason": "other"})
		status := model.TicketStatusInProgress
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("hr-manager"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("hr"), Inputs: &inputs})
			return err
		})
		if api, ok := kferr.As(err); !ok || api.Code != "validation_failed" {
			t.Fatalf("other reason without detail error=%v", err)
		}
		inputs = pf14CompleteInputs(fields, map[string]any{"start-date": "2025-02-10", "reason": "other", "detail": "Family event"})
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("hr-manager"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("hr"), Inputs: &inputs})
			return err
		})
		if err != nil {
			t.Fatalf("other reason with detail: %v", err)
		}
	})
}
