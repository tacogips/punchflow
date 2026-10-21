package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// WH-01 / PF-02: the event of an approval follows the acted-on step's action
// type, not whether the quorum was reached.
func TestApprovalEventFollowsActedStepNotQuorum(t *testing.T) {
	cases := []struct {
		name   string
		action string
		number int
		pct    int
		event  string
	}{
		{"approve number", model.ActionTypeApprove, 2, 0, model.WebhookEventTicketApproved},
		{"approve percent", model.ActionTypeApprove, 0, 100, model.WebhookEventTicketApproved},
		{"confirm number", model.ActionTypeConfirm, 2, 0, model.WebhookEventTicketConfirmed},
		{"confirm percent", model.ActionTypeConfirm, 0, 100, model.WebhookEventTicketConfirmed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := pf14Setup(t)
			step := compatUserStep("quorum", 1, tc.action, "sales-manager", "finance-manager")
			step.RequiredApprovalsNumber, step.RequiredApprovalsPercent = tc.number, tc.pct
			compatSetRoute(t, e, "expense-claim", step)
			id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)

			first := compatApprove(t, e, "sales-manager", id)
			compatAssertTypes(t, "first (partial) action", first, tc.event)
			if got := e.ticket(id).Status; got != model.TicketStatusInProgress {
				t.Fatalf("status after partial action=%s", got)
			}
			second := compatApprove(t, e, "finance-manager", id)
			compatAssertTypes(t, "final action", second, tc.event, model.WebhookEventTicketCompleted)
			if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
				t.Fatalf("status after final action=%s", got)
			}
			if ticket := e.ticket(id); ticket.CompletedAt == nil || ticket.ClosedAt == nil || !ticket.CompletedAt.Equal(*ticket.ClosedAt) {
				t.Fatalf("final %s completion timestamps completedAt=%v closedAt=%v, want equal non-nil values", tc.action, ticket.CompletedAt, ticket.ClosedAt)
			}
		})
	}
}

// WH-02 / PF-01: a route that needs no user action completes at submission and
// emits ticket_completed, for direct creation, draft submission and
// resubmission.
func TestSubmissionCompletesRouteWithoutUserAction(t *testing.T) {
	noAction := func(e *pf14TestEnv) {
		compatSetRoute(t, e, "expense-claim", compatUserStep("fyi", 1, model.ActionTypeNone, "sales-manager"))
	}
	assertCompleted := func(t *testing.T, e *pf14TestEnv, id string) {
		t.Helper()
		ticket := e.ticket(id)
		if ticket.Status != model.TicketStatusCompleted || ticket.CompletedAt == nil || ticket.ClosedAt == nil || ticket.OpenedAt == nil || !ticket.CompletedAt.Equal(*ticket.ClosedAt) {
			t.Fatalf("ticket status=%s completedAt=%v closedAt=%v openedAt=%v, want completion time", ticket.Status, ticket.CompletedAt, ticket.ClosedAt, ticket.OpenedAt)
		}
		steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range steps {
			if step.Status != model.StepStatusCompleted {
				t.Fatalf("step %v status=%s, want completed", step.RouteStepCode, step.Status)
			}
		}
	}

	t.Run("direct create", func(t *testing.T) {
		e := pf14Setup(t)
		noAction(e)
		id, events := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatAssertTypes(t, "create", events, model.WebhookEventTicketCreated, model.WebhookEventTicketOpened, model.WebhookEventTicketCompleted)
		assertCompleted(t, e, id)
	})
	t.Run("draft submission", func(t *testing.T) {
		e := pf14Setup(t)
		noAction(e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		events, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
		if err != nil {
			t.Fatal(err)
		}
		compatAssertTypes(t, "submit", events, model.WebhookEventTicketOpened, model.WebhookEventTicketCompleted)
		assertCompleted(t, e, id)
	})
	t.Run("resubmission after rejection", func(t *testing.T) {
		e := pf14Setup(t)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		e.apply(func(tx *store.Tx) error {
			_, err := e.engine.Withdraw(e.ctx, tx, e.actor("sales-rep"), id)
			return err
		})
		noAction(e)
		events, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
		if err != nil {
			t.Fatal(err)
		}
		compatAssertTypes(t, "resubmit", events, model.WebhookEventTicketOpened, model.WebhookEventTicketCompleted)
		assertCompleted(t, e, id)
	})
	t.Run("every group skipped", func(t *testing.T) {
		e := pf14Setup(t)
		skipped := compatUserStep("skipped", 1, model.ActionTypeApprove, "sales-manager")
		skipped.RouteStepCondition = &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll}
		compatSetRoute(t, e, "expense-claim", skipped)
		id, events := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatAssertTypes(t, "create", events, model.WebhookEventTicketCreated, model.WebhookEventTicketOpened, model.WebhookEventTicketCompleted)
		if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
			t.Fatalf("status=%s, want completed", got)
		}
	})
}

// PF-01: skipped and no-action groups in front of an actionable group are
// traversed on submission.
func TestSubmissionTraversesSkippedAndAutomaticGroups(t *testing.T) {
	e := pf14Setup(t)
	skipped := compatUserStep("skipped", 1, model.ActionTypeApprove, "sales-manager")
	skipped.RouteStepCondition = &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll}
	compatSetRoute(t, e, "expense-claim",
		skipped,
		compatUserStep("circulate", 2, model.ActionTypeNone, "hr-manager"),
		compatUserStep("approve", 3, model.ActionTypeApprove, "finance-manager"),
	)
	id, events := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatAssertTypes(t, "create", events, model.WebhookEventTicketCreated, model.WebhookEventTicketOpened)
	ticket := e.ticket(id)
	if ticket.Status != model.TicketStatusInProgress || ticket.CurrentStep != 3 {
		t.Fatalf("status=%s currentStep=%d, want in_progress at group 3", ticket.Status, ticket.CurrentStep)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, step := range steps {
		statuses[*step.RouteStepCode] = step.Status
	}
	want := map[string]string{"skipped": model.StepStatusSkipped, "circulate": model.StepStatusCompleted, "approve": model.StepStatusInProgress}
	for code, status := range want {
		if statuses[code] != status {
			t.Errorf("step %s status=%s, want %s (all=%v)", code, statuses[code], status, statuses)
		}
	}
	events = compatApprove(t, e, "finance-manager", id)
	compatAssertTypes(t, "approve", events, model.WebhookEventTicketApproved, model.WebhookEventTicketCompleted)
}
