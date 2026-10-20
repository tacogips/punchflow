package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestPurchaseConditionsAndNomination(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "purchase-request")
	inputs := pf14CompleteInputs(fields, map[string]any{
		"amount": 60000, "item": "Laptop", "urgent": []string{"yes"},
		"reviewer": e.user("engineer"), "approving-team": e.team("platform"),
	})
	status := model.TicketStatusInProgress
	var ticketID string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		ticketID, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{
			Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs,
			Approvers: []ApproverInput{{RouteStepCode: "purchase-nominee", UserIDs: []string{e.user("eng-manager")}}},
		})
		return err
	})
	if err != nil {
		t.Fatalf("create purchase request: %v", err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]model.TicketStep{}
	for _, step := range steps {
		if step.RouteStepCode != nil {
			byCode[*step.RouteStepCode] = step
		}
	}
	if !byCode["purchase-urgent"].Skip || byCode["purchase-urgent"].Status != model.StepStatusSkipped {
		t.Errorf("urgent step should be skipped for urgent=yes: %+v", byCode["purchase-urgent"])
	}
	if byCode["purchase-large"].Status == model.StepStatusSkipped {
		t.Errorf("large amount step should be included for amount=60000: %+v", byCode["purchase-large"])
	}

	missingNomineeErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if api, ok := kferr.As(missingNomineeErr); !ok || api.Code != "validation_failed" {
		t.Errorf("missing nominee error=%v, want validation_failed", missingNomineeErr)
	}

	budgetErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("eng-manager"), ticketID, nil)
		return err
	})
	if api, ok := kferr.As(budgetErr); !ok || api.Code != "input_required_before_approve" {
		t.Errorf("approve without budget-code error=%v, want input_required_before_approve", budgetErr)
	}
}

func TestLeaveAssigneeCustomizableAndAuthorConfirm(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "leave-request")
	inputs := pf14CompleteInputs(fields, map[string]any{"start-date": "2025-02-10", "reason": "vacation"})
	status := model.TicketStatusInProgress
	var ticketID string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		ticketID, _, err = e.engine.Create(e.ctx, tx, e.actor("hr-manager"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("hr"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatalf("create leave request: %v", err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 2 || steps[0].RouteStepCode == nil || *steps[0].RouteStepCode != "leave-manager" {
		t.Fatalf("leave route steps=%+v, first step should be leave-manager", steps)
	}
	managerAssignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(managerAssignees) != 1 || managerAssignees[0].UserID != e.user("ceo") {
		t.Fatalf("leave manager assignees=%+v, want CEO", managerAssignees)
	}
	assignmentErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("ceo"), ticketID, nil)
		return err
	})
	if api, ok := kferr.As(assignmentErr); !ok || api.Code != "assignment_required_before_approve" {
		t.Fatalf("CEO approval without delegate error=%v, want assignment_required_before_approve", assignmentErr)
	}
	delegates := []ApproverInput{{RouteStepCode: "leave-delegate", UserIDs: []string{e.user("hr-manager")}}}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Update(e.ctx, tx, e.actor("ceo"), ticketID, UpdateInput{Approvers: &delegates})
		return err
	}); err != nil {
		t.Fatalf("set delegate: %v", err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("ceo"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("CEO approve after setting delegate: %v", err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("hr-manager"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("delegate approve: %v", err)
	}
	steps, err = store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 4 || steps[2].Status != model.StepStatusCompleted {
		t.Fatalf("none action step did not auto-complete: %+v", steps)
	}
	var events []model.WebhookEvent
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Approve(e.ctx, tx, e.actor("hr-manager"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("author confirmation: %v", err)
	}
	if len(events) != 2 || events[0].Type != model.WebhookEventTicketConfirmed || events[1].Type != model.WebhookEventTicketCompleted {
		t.Fatalf("author confirmation events=%+v, want confirmed then completed", events)
	}
	if got := e.ticket(ticketID).Status; got != model.TicketStatusCompleted {
		t.Fatalf("ticket status=%s, want completed", got)
	}
}
