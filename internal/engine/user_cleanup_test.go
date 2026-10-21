package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func cleanupDeleteUser(t *testing.T, e *pf14TestEnv, userCode string) []model.WebhookEvent {
	t.Helper()
	var events []model.WebhookEvent
	e.apply(func(tx *store.Tx) error {
		// The handler deactivates the user before the cleanup runs.
		if _, err := tx.ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, model.UserStatusDeactivated, e.user(userCode)); err != nil {
			return err
		}
		var err error
		events, err = e.engine.CleanupDeletedUser(e.ctx, tx, e.actor("admin"), e.user(userCode))
		return err
	})
	return events
}

func cleanupStep(t *testing.T, e *pf14TestEnv, ticketID, routeStepCode string) model.TicketStep {
	t.Helper()
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.RouteStepCode != nil && *step.RouteStepCode == routeStepCode {
			return step
		}
	}
	t.Fatalf("ticket has no step %q", routeStepCode)
	return model.TicketStep{}
}

func cleanupAssigneeIDs(t *testing.T, e *pf14TestEnv, step model.TicketStep) []string {
	t.Helper()
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), step.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, as := range assignees {
		ids = append(ids, as.UserID)
	}
	return ids
}

func cleanupWithFallback(step model.RouteStep, fallback string) model.RouteStep {
	step.FallbackType = &fallback
	return step
}

// INF-79: a current step whose only assignee is deleted and whose fallback is "skip"
// is skipped; with nothing left to do the ticket completes like a submission
// that needs no user action.
func TestCleanupDeletedUserSkipFallbackCompletesTicket(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		cleanupWithFallback(compatUserStep("only", 1, model.ActionTypeApprove, "finance-manager"), model.FallbackSkip))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress || got.CurrentStep != 1 {
		t.Fatalf("precondition: status=%s current=%d", got.Status, got.CurrentStep)
	}
	events := cleanupDeleteUser(t, e, "finance-manager")
	compatAssertTypes(t, "cleanup", events, model.WebhookEventTicketCompleted)
	if got := e.ticket(id); got.Status != model.TicketStatusCompleted || got.CompletedAt == nil || got.ClosedAt == nil {
		t.Fatalf("ticket status=%s completedAt=%v, want completed", got.Status, got.CompletedAt)
	}
	step := cleanupStep(t, e, id, "only")
	if step.Status != model.StepStatusSkipped || !step.Skip || !step.Fallback || step.FallbackResult == nil || *step.FallbackResult != model.FallbackSkip {
		t.Fatalf("step = %+v, want skipped by fallback", step)
	}
}

func TestCleanupArchiveClosesTicketAndClearsSubStatus(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if got := pf15SubStatusCode(t, e, id); got != "under-review" {
		t.Fatalf("sub-status before cleanup=%q", got)
	}
	cleanupDeleteUser(t, e, "sales-rep")
	ticket := e.ticket(id)
	if ticket.Status != model.TicketStatusArchived || ticket.ClosedAt == nil || ticket.ArchivedAt == nil || !ticket.ClosedAt.Equal(*ticket.ArchivedAt) {
		t.Fatalf("cleanup archive ticket=%+v, want matching archive/closed times", ticket)
	}
	if got := pf15SubStatusCode(t, e, id); got != "" {
		t.Fatalf("archived ticket retained sub-status %q", got)
	}
}

// A future step that loses all its assignees is resolved immediately, so the
// ticket does not stall when it becomes the current step.
func TestCleanupDeletedUserSkipsFutureStepWithoutAssignees(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("first", 1, model.ActionTypeApprove, "accountant"),
		cleanupWithFallback(compatUserStep("second", 2, model.ActionTypeApprove, "finance-manager"), model.FallbackSkip),
		compatUserStep("third", 3, model.ActionTypeApprove, "ceo"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if events := cleanupDeleteUser(t, e, "finance-manager"); len(events) != 0 {
		t.Fatalf("cleanup events = %v, want none", compatTypes(events))
	}
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress || got.CurrentStep != 1 {
		t.Fatalf("status=%s current=%d, want in_progress at group 1", got.Status, got.CurrentStep)
	}
	if step := cleanupStep(t, e, id, "second"); step.Status != model.StepStatusSkipped {
		t.Fatalf("second step status = %s, want skipped", step.Status)
	}
	compatApprove(t, e, "accountant", id)
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress || got.CurrentStep != 3 {
		t.Fatalf("after approval status=%s current=%d, want in_progress at group 3", got.Status, got.CurrentStep)
	}
}

// When the remaining assignees of the current step already satisfy its
// approval threshold the step completes and the ticket advances.
func TestCleanupDeletedUserCompletesStepWhenRemainingAssigneesSuffice(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("both", 1, model.ActionTypeApprove, "accountant", "finance-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "accountant", id)
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress {
		t.Fatalf("precondition: status=%s", got.Status)
	}
	events := cleanupDeleteUser(t, e, "finance-manager")
	compatAssertTypes(t, "cleanup", events, model.WebhookEventTicketCompleted)
	if got := e.ticket(id); got.Status != model.TicketStatusCompleted {
		t.Fatalf("ticket status = %s, want completed", got.Status)
	}
}

// A configured fallback that finds approvers replaces the deleted assignee.
func TestCleanupDeletedUserFallbackAssignsReplacement(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		cleanupWithFallback(compatUserStep("only", 1, model.ActionTypeApprove, "finance-manager"), model.FallbackHigherManager))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	events := cleanupDeleteUser(t, e, "finance-manager")
	if len(events) != 0 {
		t.Fatalf("cleanup events = %v, want none", compatTypes(events))
	}
	step := cleanupStep(t, e, id, "only")
	assignees := cleanupAssigneeIDs(t, e, step)
	if len(assignees) == 0 {
		t.Fatalf("fallback assigned nobody; step = %+v", step)
	}
	for _, uid := range assignees {
		if uid == e.user("finance-manager") {
			t.Fatal("deleted user is still assigned")
		}
	}
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress {
		t.Fatalf("ticket status = %s, want in_progress", got.Status)
	}
	if step.Status != model.StepStatusInProgress {
		t.Fatalf("step status = %s, want in_progress", step.Status)
	}
}

// Without a configured fallback nothing can replace the assignee, so the step
// keeps waiting (a definition like this is rejected at submission, but a user
// deletion cannot be rejected).
func TestCleanupDeletedUserWithoutFallbackKeepsStepWaiting(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("only", 1, model.ActionTypeApprove, "finance-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if events := cleanupDeleteUser(t, e, "finance-manager"); len(events) != 0 {
		t.Fatalf("cleanup events = %v, want none", compatTypes(events))
	}
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress || got.CurrentStep != 1 {
		t.Fatalf("status=%s current=%d, want in_progress at group 1", got.Status, got.CurrentStep)
	}
	step := cleanupStep(t, e, id, "only")
	if step.Status != model.StepStatusInProgress || len(cleanupAssigneeIDs(t, e, step)) != 0 {
		t.Fatalf("step = %+v, want waiting without assignees", step)
	}
}

// The deleted user may still lead the author's team; the fallback must walk
// past them instead of re-assigning the step to the deleted user.
func TestCleanupDeletedUserFallbackSkipsDeletedLeader(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		cleanupWithFallback(compatUserStep("only", 1, model.ActionTypeApprove, "sales-manager"), model.FallbackHigherManager))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	cleanupDeleteUser(t, e, "sales-manager")
	step := cleanupStep(t, e, id, "only")
	for _, uid := range cleanupAssigneeIDs(t, e, step) {
		if uid == e.user("sales-manager") {
			t.Fatal("deleted leader was chosen as fallback assignee")
		}
	}
	if got := e.ticket(id); got.Status != model.TicketStatusInProgress {
		t.Fatalf("ticket status = %s, want in_progress", got.Status)
	}
}
