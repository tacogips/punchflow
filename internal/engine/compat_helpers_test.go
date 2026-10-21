package engine

import (
	"encoding/json"
	"reflect"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// compatUserStep builds a static user route step.
func compatUserStep(code string, order int, action string, users ...string) model.RouteStep {
	return model.RouteStep{
		Code: code, StepOrder: order, Title: code, StepType: model.StepTypeUser, ActionType: action,
		UserCodes: users, ManagerSelectionType: model.ManagerSelectionClosest,
	}
}

// compatSetRoute replaces the steps of the route used by the workflow.
func compatSetRoute(t *testing.T, e *pf14TestEnv, workflowCode string, steps ...model.RouteStep) {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), workflowCode)
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	routeID, err := store.RouteIDByCode(e.ctx, e.db.Q(), def.RouteCode)
	if err != nil {
		t.Fatal(err)
	}
	route, err := store.RouteDefByID(e.ctx, e.db.Q(), routeID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range steps {
		steps[i].ID = "compat-step-" + steps[i].Code
	}
	route.Steps = steps
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE routes SET definition_json=? WHERE id=?`, string(data), routeID)
		return err
	})
}

// compatSetWorkflow sets one JSON path of a workflow definition.
func compatSetWorkflow(t *testing.T, e *pf14TestEnv, workflowCode, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE workflows SET definition_json=json_set(definition_json,?,json(?)) WHERE code=?`, path, string(raw), workflowCode)
		return err
	})
}

// compatCreateExpense creates an expense-claim ticket and returns the events.
func compatCreateExpense(t *testing.T, e *pf14TestEnv, actorCode, teamCode, status string, approvers []ApproverInput) (string, []model.WebhookEvent, error) {
	t.Helper()
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 5000, "purpose": "compat"})
	var id string
	var events []model.WebhookEvent
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, events, err = e.engine.Create(e.ctx, tx, e.actor(actorCode), CreateInput{
			Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team(teamCode), Inputs: &inputs, Approvers: approvers,
		})
		return err
	})
	return id, events, err
}

func compatMustCreate(t *testing.T, e *pf14TestEnv, actorCode, status string) (string, []model.WebhookEvent) {
	t.Helper()
	id, events, err := compatCreateExpense(t, e, actorCode, "sales", status, nil)
	if err != nil {
		t.Fatalf("create %s ticket: %v", status, err)
	}
	return id, events
}

func compatUpdate(e *pf14TestEnv, actorCode, id string, in UpdateInput) ([]model.WebhookEvent, error) {
	var events []model.WebhookEvent
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Update(e.ctx, tx, e.actor(actorCode), id, in)
		return err
	})
	return events, err
}

func compatApprove(t *testing.T, e *pf14TestEnv, actorCode, id string) []model.WebhookEvent {
	t.Helper()
	var events []model.WebhookEvent
	e.apply(func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Approve(e.ctx, tx, e.actor(actorCode), id, nil)
		return err
	})
	return events
}

func compatTypes(events []model.WebhookEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

func compatAssertTypes(t *testing.T, label string, events []model.WebhookEvent, want ...string) {
	t.Helper()
	got := compatTypes(events)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s events=%v, want %v", label, got, want)
	}
}

func compatStr(value string) *string { return &value }
