package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestSubmitMultiStepApproveCompletes(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	for _, code := range []string{"sales-manager", "finance-manager"} {
		actor := e.actor(code)
		e.apply(func(tx *store.Tx) error { _, err := e.engine.Approve(e.ctx, tx, actor, id, nil); return err })
	}
	ticket := e.ticket(id)
	if ticket.Status != model.TicketStatusCompleted {
		t.Fatalf("status=%s", ticket.Status)
	}
	if ticket.CompletedAt == nil || ticket.ClosedAt == nil || !ticket.CompletedAt.Equal(*ticket.ClosedAt) {
		t.Fatalf("completion timestamps completedAt=%v closedAt=%v, want same completion time", ticket.CompletedAt, ticket.ClosedAt)
	}
	if ticket.TicketNumber == nil || *ticket.TicketNumber != "EXP-2025-1" {
		t.Fatalf("ticket number=%v", ticket.TicketNumber)
	}
}
func TestDenyIsTerminal(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Deny(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	if got := e.ticket(id).Status; got != model.TicketStatusDenied {
		t.Fatalf("status=%s", got)
	}
	if ticket := e.ticket(id); ticket.ClosedAt != nil || ticket.DeniedAt == nil {
		t.Fatalf("denial timestamps deniedAt=%v closedAt=%v, want closedAt null", ticket.DeniedAt, ticket.ClosedAt)
	}
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	if api, ok := kferr.As(err); !ok || api.Code != "invalid_state" {
		t.Fatalf("approve after deny error=%v", err)
	}
}
func TestSendBackToApplicantAndResubmit(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.SendBack(e.ctx, tx, e.actor("finance-manager"), id, 0, nil)
		return err
	})
	if got := e.ticket(id).Status; got != model.TicketStatusRejected {
		t.Fatalf("status=%s", got)
	}
	if ticket := e.ticket(id); ticket.ClosedAt != nil {
		t.Fatalf("withdrawn ticket closedAt=%v, want null", ticket.ClosedAt)
	}
	status := model.TicketStatusInProgress
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Update(e.ctx, tx, e.actor("sales-rep"), id, UpdateInput{Status: &status})
		return err
	})
	if err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if got := e.ticket(id).Status; got != model.TicketStatusInProgress {
		t.Fatalf("status=%s", got)
	}
}
func TestSendBackToEarlierStep(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.SendBack(e.ctx, tx, e.actor("finance-manager"), id, 1, nil)
		return err
	})
	if got := e.ticket(id).CurrentStep; got != 1 {
		t.Fatalf("currentStep=%d", got)
	}
}

func TestSendBackPreservesUnactivatedCustomStepCreationTimestamp(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
		pf15RouteStep("final", 3, model.ActionTypeApprove, "", false, "ceo"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "sales-manager", id)
	two := 2
	_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "Future custom", ActionType: model.ActionTypeApprove, UserIDs: []string{e.user("hr-manager")}, AfterStepOrder: &two})
	})
	if err != nil {
		t.Fatal(err)
	}
	custom := pf15StepByTitle(t, e, id, "Future custom")
	assignments, err := store.EngStepAssignees(e.ctx, e.db.Q(), custom.ID)
	if err != nil || len(assignments) != 1 || assignments[0].AssignedAt == nil {
		t.Fatalf("custom assignments before send-back: %+v err=%v", assignments, err)
	}
	createdAssignmentAt := *assignments[0].AssignedAt
	_, err = pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.SendBack(e.ctx, tx, e.actor("finance-manager"), id, 1, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	assignments, err = store.EngStepAssignees(e.ctx, e.db.Q(), custom.ID)
	if err != nil || len(assignments) != 1 || assignments[0].AssignedAt == nil || !assignments[0].AssignedAt.Equal(createdAssignmentAt) {
		t.Fatalf("send-back changed unactivated custom-step AssignedAt: %+v err=%v", assignments, err)
	}
}
func TestWithdraw(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	var events []model.WebhookEvent
	e.apply(func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Withdraw(e.ctx, tx, e.actor("sales-rep"), id)
		return err
	})
	if got := e.ticket(id).Status; got != model.TicketStatusRejected {
		t.Fatalf("status=%s", got)
	}
	if len(events) != 1 || events[0].Type != model.WebhookEventTicketWithdrawn {
		t.Fatalf("events=%+v", events)
	}
}
func TestParallelAllAndAny(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 200000, true)
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	ticket := e.ticket(id)
	if ticket.Status != model.TicketStatusInProgress {
		t.Fatalf("status=%s", ticket.Status)
	}
}
func TestProxyApproval(t *testing.T) {
	e := pf14Setup(t)
	id := e.createExpense("sales-rep", 5000, true)
	proxy := e.actor("sales-deputy")
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error { _, err := e.engine.Approve(e.ctx, tx, proxy, id, nil); return err }); err != nil {
		t.Fatalf("proxy approve: %v", err)
	}
}
func TestRouteBranching(t *testing.T) {
	e := pf14Setup(t)
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, section := range def.SectionList {
		for _, formField := range section.FormFields {
			fields[formField.Code] = formField.ID
		}
	}
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 20000000, "purpose": "client meal"})
	status := model.TicketStatusInProgress
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if api, ok := kferr.As(err); !ok || api.Code != "validation_failed" {
		t.Fatalf("err=%v", err)
	}
}
