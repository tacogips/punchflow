package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// INF-12: steps skipped at submission must stay skipped when a ticket is sent back
// over them; they must not become approvable again.
func TestSendBackKeepsSkippedStepsSkipped(t *testing.T) {
	e := pf14Setup(t)
	skipped := compatUserStep("middle", 2, model.ActionTypeApprove, "finance-manager")
	skipped.RouteStepCondition = &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditionalSkip}
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("first", 1, model.ActionTypeApprove, "accountant"),
		skipped,
		compatUserStep("last", 3, model.ActionTypeApprove, "ceo"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "accountant", id)
	if got := e.ticket(id); got.CurrentStep != 3 {
		t.Fatalf("precondition: current step = %d, want 3 (middle step skipped)", got.CurrentStep)
	}
	if _, err := pf15SendBack(e, "ceo", id, 1); err != nil {
		t.Fatal(err)
	}
	if step := cleanupStep(t, e, id, "middle"); step.Status != model.StepStatusSkipped || !step.Skip {
		t.Fatalf("middle step = %s skip=%v after send back, want skipped", step.Status, step.Skip)
	}
	if step := cleanupStep(t, e, id, "first"); step.Status != model.StepStatusInProgress || step.CompletedAt != nil {
		t.Fatalf("first step = %s completedAt=%v, want in_progress and not completed", step.Status, step.CompletedAt)
	}
	compatApprove(t, e, "accountant", id)
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress || got.CurrentStep != 3 {
		t.Fatalf("after re-approval status=%s current=%d, want in_progress at group 3", got.Status, got.CurrentStep)
	}
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("ceo"), id, nil)
		return err
	})
	if got := e.ticket(id); got.Status != model.TicketStatusCompleted {
		t.Fatalf("ticket status = %s, want completed", got.Status)
	}
}

// Sending back to a group whose steps were all skipped continues with the
// next group that needs an approval instead of leaving nobody to act.
func TestSendBackToFullySkippedGroupAdvances(t *testing.T) {
	e := pf14Setup(t)
	skipped := compatUserStep("skipped", 1, model.ActionTypeApprove, "finance-manager")
	skipped.RouteStepCondition = &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditionalSkip}
	compatSetRoute(t, e, "expense-claim",
		skipped,
		compatUserStep("second", 2, model.ActionTypeApprove, "accountant"),
		compatUserStep("third", 3, model.ActionTypeApprove, "ceo"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "accountant", id)
	if _, err := pf15SendBack(e, "ceo", id, 1); err != nil {
		t.Fatal(err)
	}
	got := e.ticket(id)
	if got.Status != model.TicketStatusInProgress || got.CurrentStep != 2 {
		t.Fatalf("status=%s current=%d, want in_progress at group 2", got.Status, got.CurrentStep)
	}
	if step := cleanupStep(t, e, id, "second"); step.Status != model.StepStatusInProgress {
		t.Fatalf("second step = %s, want in_progress", step.Status)
	}
}
