package engine

import (
	"reflect"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf15Cancel(e *pf14TestEnv, actor, id string) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.CancelApproval(e.ctx, tx, e.actor(actor), id, nil)
	})
}

func pf15Raise(e *pf14TestEnv, actor, id string, to *int) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.Raise(e.ctx, tx, e.actor(actor), id, to)
	})
}

func pf15ThreeStepRoute(t *testing.T, e *pf14TestEnv) {
	t.Helper()
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "finance-check", true, "finance-manager"),
		pf15RouteStep("final", 3, model.ActionTypeApprove, "", false, "ceo"),
	)
}

// PF-19: the approver cancels an approval while the next step has not acted.
func TestCancelApprovalReopensTheStep(t *testing.T) {
	e := pf14Setup(t)
	pf15ThreeStepRoute(t, e)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "sales-manager", id)
	if got := e.ticket(id).CurrentStep; got != 2 {
		t.Fatalf("current step=%d, want 2", got)
	}
	if !pf15Controls(t, e, "sales-manager", id).CancelApproval {
		t.Fatal("controls do not offer cancellation to the approver")
	}
	events, err := pf15Cancel(e, "sales-manager", id)
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "cancel", events, model.WebhookEventTicketUpdated)
	ticket := e.ticket(id)
	if ticket.Status != model.TicketStatusInProgress || ticket.CurrentStep != 1 {
		t.Fatalf("ticket after cancel: status=%s step=%d, want in_progress/1", ticket.Status, ticket.CurrentStep)
	}
	if got := pf15StepByTitle(t, e, id, "review").Status; got != model.StepStatusInProgress {
		t.Fatalf("review step=%s, want in_progress", got)
	}
	if got := pf15StepByTitle(t, e, id, "finance").Status; got != model.StepStatusPending {
		t.Fatalf("finance step=%s, want pending", got)
	}
	if got := pf15SubStatusCode(t, e, id); got != "under-review" {
		t.Fatalf("sub-status after cancel=%q, want under-review", got)
	}
	// The finance approver can no longer act, the canceller can approve again.
	if _, err = pf15Approve(e, "finance-manager", id); err == nil {
		t.Fatal("finance-manager approved a step that is not current")
	}
	compatApprove(t, e, "sales-manager", id)
	if got := e.ticket(id).CurrentStep; got != 2 {
		t.Fatalf("current step after re-approval=%d, want 2", got)
	}
}

func TestCancelApprovalRefusals(t *testing.T) {
	t.Run("next step already acted", func(t *testing.T) {
		e := pf14Setup(t)
		pf15ThreeStepRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		compatApprove(t, e, "finance-manager", id)
		_, err := pf15Cancel(e, "sales-manager", id)
		pf15WantCode(t, "cancel after next step acted", err, "invalid_state")
		if pf15Controls(t, e, "sales-manager", id).CancelApproval {
			t.Fatal("controls offer cancellation after the next step acted")
		}
		// The latest approver may still cancel their own approval.
		if _, err = pf15Cancel(e, "finance-manager", id); err != nil {
			t.Fatalf("latest approver cancel: %v", err)
		}
	})
	t.Run("not an approver", func(t *testing.T) {
		e := pf14Setup(t)
		pf15ThreeStepRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		_, err := pf15Cancel(e, "ceo", id)
		pf15WantCode(t, "cancel by non-approver", err, "invalid_state")
		_, err = pf15Cancel(e, "sales-rep", id)
		pf15WantCode(t, "cancel by applicant", err, "invalid_state")
	})
	t.Run("workflow disallows", func(t *testing.T) {
		e := pf14Setup(t)
		pf15ThreeStepRoute(t, e)
		compatSetWorkflow(t, e, "expense-claim", "$.approvalCancellable", false)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		_, err := pf15Cancel(e, "sales-manager", id)
		pf15WantCode(t, "cancel when disabled", err, "invalid_state")
	})
	t.Run("completed ticket", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetRoute(t, e, "expense-claim", pf15RouteStep("only", 1, model.ActionTypeApprove, "", false, "sales-manager"))
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		_, err := pf15Cancel(e, "sales-manager", id)
		pf15WantCode(t, "cancel completed", err, "invalid_state")
	})
}

func TestCancelApprovalUsesActiveProxyAndRejectsTwoGroupsLater(t *testing.T) {
	t.Run("active proxy can cancel assigned approver action", func(t *testing.T) {
		e := pf14Setup(t)
		pf15ThreeStepRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		if _, err := pf15Cancel(e, "sales-deputy", id); err != nil {
			t.Fatalf("active proxy could not cancel assigned manager approval: %v", err)
		}
		if got := e.ticket(id).CurrentStep; got != 1 {
			t.Fatalf("currentStep=%d, want reopened group 1", got)
		}
	})
	t.Run("two groups later cannot cancel", func(t *testing.T) {
		e := pf14Setup(t)
		pf15ThreeStepRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		compatApprove(t, e, "finance-manager", id)
		if got := e.ticket(id).CurrentStep; got != 3 {
			t.Fatalf("currentStep=%d, want 3", got)
		}
		_, err := pf15Cancel(e, "sales-manager", id)
		pf15WantCode(t, "cancel two groups later", err, "invalid_state")
	})
}

// PF-19: an approver of a later step that allows raising pulls the ticket up.
func TestRaiseSkipsAheadToTheCallersStep(t *testing.T) {
	e := pf14Setup(t)
	pf15ThreeStepRoute(t, e)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	controls := pf15Controls(t, e, "finance-manager", id)
	if !controls.Raise || !reflect.DeepEqual(controls.RaiseTargetStepOrders, []int{2}) {
		t.Fatalf("finance-manager controls=%+v, want raise to [2]", controls)
	}
	if pf15Controls(t, e, "ceo", id).Raise {
		t.Fatal("ceo may raise although the final step does not allow raising")
	}
	if pf15Controls(t, e, "sales-manager", id).Raise {
		t.Fatal("the current approver has nothing to raise to")
	}
	events, err := pf15Raise(e, "finance-manager", id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("raise events=%+v, want no webhook events", events)
	}
	if got := e.ticket(id).CurrentStep; got != 2 {
		t.Fatalf("current step=%d, want exactly the next group (2)", got)
	}
	if got := pf15StepByTitle(t, e, id, "review").Status; got != model.StepStatusRaised {
		t.Fatalf("review step=%s, want raised", got)
	}
	if got := pf15StepByTitle(t, e, id, "finance").Status; got != model.StepStatusInProgress {
		t.Fatalf("finance step=%s, want in_progress", got)
	}
	if got := pf15SubStatusCode(t, e, id); got != "finance-check" {
		t.Fatalf("sub-status after raise=%q, want finance-check", got)
	}
	if _, err = pf15Approve(e, "sales-manager", id); err == nil {
		t.Fatal("the bypassed approver can still approve")
	}
	compatApprove(t, e, "finance-manager", id)
	compatApprove(t, e, "ceo", id)
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("status=%s, want completed", got)
	}
}

func TestRaiseRefusalsAndExplicitTarget(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("mid", 2, model.ActionTypeApprove, "", true, "finance-manager"),
		pf15RouteStep("last", 3, model.ActionTypeApprove, "", true, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if got := pf15Controls(t, e, "finance-manager", id).RaiseTargetStepOrders; !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("raise targets=%v, want [2 3]", got)
	}
	_, err := pf15Raise(e, "sales-manager", id, nil)
	pf15WantCode(t, "raise by current approver", err, "invalid_state")
	bad := 1
	_, err = pf15Raise(e, "finance-manager", id, &bad)
	pf15WantCode(t, "raise to an ineligible step", err, "validation_failed")
	last := 3
	if _, err = pf15Raise(e, "finance-manager", id, &last); err != nil {
		t.Fatal(err)
	}
	if got := e.ticket(id).CurrentStep; got != 2 {
		t.Fatalf("current step=%d, want exactly the next group (2)", got)
	}
	if got := pf15StepByTitle(t, e, id, "review").Status; got != model.StepStatusRaised {
		t.Fatalf("review step=%s, want raised", got)
	}
	if got := pf15StepByTitle(t, e, id, "mid").Status; got != model.StepStatusInProgress {
		t.Fatalf("mid step=%s, want in_progress", got)
	}
	compatApprove(t, e, "finance-manager", id)
	compatApprove(t, e, "finance-manager", id)
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("status=%s, want completed after approving the raised-to step", got)
	}
}

// PF-19: custom steps are inserted after the current group and run in order.
func TestAddCustomStepRunsBetweenRouteSteps(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if !pf15Controls(t, e, "sales-rep", id).AddCustomStep {
		t.Fatal("applicant is not offered custom steps")
	}
	events, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{
			Title: "Legal check", ActionType: model.ActionTypeApprove, UserIDs: []string{e.user("hr-manager")}, Instruction: compatStr("check the contract"),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "add custom step", events, model.WebhookEventTicketUpdated)
	steps := pf15Steps(t, e, id)
	var titles []string
	for i, step := range steps {
		titles = append(titles, *step.Title)
		if step.DisplayOrder != i+1 {
			t.Errorf("step %s display order=%d, want %d", *step.Title, step.DisplayOrder, i+1)
		}
	}
	if !reflect.DeepEqual(titles, []string{"review", "Legal check", "finance"}) {
		t.Fatalf("step titles=%v", titles)
	}
	custom := pf15StepByTitle(t, e, id, "Legal check")
	if custom.StepOrder != 2 || custom.RouteStepID != nil || custom.AuthorID == nil || *custom.AuthorID != e.user("sales-rep") || custom.Instruction == nil {
		t.Fatalf("custom step=%+v", custom)
	}
	customAssignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(customAssignees) != 1 || customAssignees[0].AssignedAt == nil || !customAssignees[0].AssignedAt.Equal(custom.CreatedAt) {
		t.Fatalf("custom-step assignees=%+v; assignment date should equal creation time %v", customAssignees, custom.CreatedAt)
	}
	if got := pf15StepByTitle(t, e, id, "finance").StepOrder; got != 3 {
		t.Fatalf("finance step order=%d, want 3", got)
	}
	compatApprove(t, e, "sales-manager", id)
	if _, err = pf15Approve(e, "finance-manager", id); err == nil {
		t.Fatal("finance approved before the custom step")
	}
	compatApprove(t, e, "hr-manager", id)
	if got := e.ticket(id).CurrentStep; got != 3 {
		t.Fatalf("current step=%d, want 3", got)
	}
	compatApprove(t, e, "finance-manager", id)
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("status=%s, want completed", got)
	}
}

func TestAddCustomStepEligibilityAndValidation(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	add := func(actor string, in CustomStepInput) error {
		_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.AddCustomStep(e.ctx, tx, e.actor(actor), id, in)
		})
		return err
	}
	valid := CustomStepInput{Title: "extra", ActionType: model.ActionTypeConfirm, UserIDs: []string{e.user("hr-manager")}}
	pf15WantCode(t, "unrelated user", add("hr-manager", valid), "invisible_ticket")
	pf15WantCode(t, "later approver", add("finance-manager", valid), "missing_permission")
	noTitle := valid
	noTitle.Title = " "
	pf15WantCode(t, "empty title", add("sales-rep", noTitle), "validation_failed")
	noUsers := valid
	noUsers.UserIDs = nil
	pf15WantCode(t, "no approvers", add("sales-rep", noUsers), "validation_failed")
	badAction := valid
	badAction.ActionType = "unsupported"
	pf15WantCode(t, "bad action", add("sales-rep", badAction), "validation_failed")
	unknown := valid
	unknown.UserIDs = []string{"00000000-0000-4000-8000-00000000ffff"}
	pf15WantCode(t, "unknown user", add("sales-rep", unknown), "validation_failed")
	before := 0
	misplaced := valid
	misplaced.AfterStepOrder = &before
	pf15WantCode(t, "position before the current group", add("sales-rep", misplaced), "validation_failed")
	// The current approver may add a step too.
	if err := add("sales-manager", valid); err != nil {
		t.Fatalf("current approver: %v", err)
	}
	compatSetWorkflow(t, e, "expense-claim", "$.allowCustomSteps", false)
	pf15WantCode(t, "workflow disabled", add("sales-rep", valid), "invalid_state")
	if pf15Controls(t, e, "sales-rep", id).AddCustomStep {
		t.Fatal("controls offer custom steps although the workflow disables them")
	}
}

func TestCustomStepCannotBeInsertedAfterConditionalRouteStep(t *testing.T) {
	e := pf14Setup(t)
	conditional := pf15RouteStep("conditional", 2, model.ActionTypeApprove, "", false, "finance-manager")
	conditional.RouteStepCondition = &model.RouteStepCondition{ID: "condition", ConditionType: model.StepConditionTypeConditional, CombinationType: model.StepCombinationAll}
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		conditional,
		pf15RouteStep("final", 3, model.ActionTypeApprove, "", false, "ceo"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	controls := pf15Controls(t, e, "sales-rep", id)
	for _, order := range controls.CustomStepInsertAfter {
		if order == 2 {
			t.Fatalf("custom-step controls allow insertion after conditional step: %+v", controls.CustomStepInsertAfter)
		}
	}
	order := 2
	_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "Forbidden", ActionType: model.ActionTypeApprove, UserIDs: []string{e.user("hr-manager")}, AfterStepOrder: &order})
	})
	pf15WantCode(t, "insert after conditional route step", err, "validation_failed")
}

func TestCustomStepApprovalCountCanBeBelowUnanimous(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	users := []string{e.user("hr-manager"), e.user("accountant")}
	_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "One of two", ActionType: model.ActionTypeApprove, UserIDs: users, RequiredApprovalsNumber: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	compatApprove(t, e, "sales-manager", id)
	custom := pf15StepByTitle(t, e, id, "One of two")
	if custom.RequiredApprovalsNumber != 1 || custom.RequiredApprovalsPercent != 0 {
		t.Fatalf("threshold=%d/%d", custom.RequiredApprovalsNumber, custom.RequiredApprovalsPercent)
	}
	compatApprove(t, e, "hr-manager", id)
	if got := pf15StepByTitle(t, e, id, "One of two").Status; got != model.StepStatusCompleted {
		t.Fatalf("custom status=%s, want completed after first approval", got)
	}
	if got := e.ticket(id).CurrentStep; got != 3 {
		t.Fatalf("current step=%d, want next route step 3", got)
	}
}

func TestCustomStepSupportsTeamAssignmentAndNoConfirmation(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "Team notice", ActionType: model.ActionTypeNone, TeamCodes: []string{"finance"}})
	})
	if err != nil {
		t.Fatalf("add no-confirmation team step: %v", err)
	}
	if len(pf15StepByTitle(t, e, id, "Team notice").ID) == 0 {
		t.Fatal("custom step was not stored")
	}
	compatApprove(t, e, "sales-manager", id)
	step := pf15StepByTitle(t, e, id, "Team notice")
	if step.Status != model.StepStatusCompleted {
		t.Fatalf("no-confirmation step status=%s, want completed", step.Status)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) == 0 {
		t.Fatal("team assignment did not resolve active members")
	}
}

func TestCustomNoConfirmationStepWithoutAssigneesDoesNotBlock(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "Notice", ActionType: model.ActionTypeNone})
	})
	if err != nil {
		t.Fatalf("add no-confirmation step without assignees: %v", err)
	}
	compatApprove(t, e, "sales-manager", id)
	step := pf15StepByTitle(t, e, id, "Notice")
	if step.Status != model.StepStatusCompleted {
		t.Fatalf("no-confirmation step status=%s, want completed", step.Status)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 0 {
		t.Fatalf("no-confirmation step has %d assignees, want none", len(assignees))
	}
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("ticket status=%s, want completed after auto-advancing past custom step", got)
	}
}

func TestRemoveCustomStep(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "extra", ActionType: model.ActionTypeApprove, UserIDs: []string{e.user("hr-manager")}})
	}); err != nil {
		t.Fatal(err)
	}
	custom := pf15StepByTitle(t, e, id, "extra")
	remove := func(actor, stepID string) error {
		_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.RemoveCustomStep(e.ctx, tx, e.actor(actor), id, stepID)
		})
		return err
	}
	pf15WantCode(t, "remove by another user", remove("sales-manager", custom.ID), "missing_permission")
	pf15WantCode(t, "remove a route step", remove("sales-rep", pf15StepByTitle(t, e, id, "review").ID), "invalid_state")
	pf15WantCode(t, "remove unknown step", remove("sales-rep", "missing"), "resource_not_found")
	if got := pf15Controls(t, e, "sales-rep", id).RemovableCustomStepIDs; !reflect.DeepEqual(got, []string{custom.ID}) {
		t.Fatalf("removable steps=%v, want [%s]", got, custom.ID)
	}
	if err := remove("sales-rep", custom.ID); err != nil {
		t.Fatal(err)
	}
	steps := pf15Steps(t, e, id)
	if len(steps) != 2 || steps[1].StepOrder != 2 || steps[1].DisplayOrder != 2 {
		t.Fatalf("steps after removal: %+v", steps)
	}
	compatApprove(t, e, "sales-manager", id)
	compatApprove(t, e, "finance-manager", id)
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("status=%s, want completed", got)
	}
}

// A custom step in front of an assignee_customizable step must not hide it
// from the assignment check that guards approval.
func TestCustomStepKeepsNextCustomizableStepCheck(t *testing.T) {
	e := pf14Setup(t)
	delegate := compatUserStep("delegate", 2, model.ActionTypeApprove)
	delegate.StepType = model.StepTypeAssigneeCustomizable
	delegate.UserCodes = nil
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
		delegate,
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.AddCustomStep(e.ctx, tx, e.actor("sales-rep"), id, CustomStepInput{Title: "extra", ActionType: model.ActionTypeApprove, UserIDs: []string{e.user("hr-manager")}})
	}); err != nil {
		t.Fatal(err)
	}
	compatApprove(t, e, "sales-manager", id)
	// The custom step is current; approving it must demand the nomination of the
	// customizable step that follows it.
	_, err := pf15Approve(e, "hr-manager", id)
	pf15WantCode(t, "approve before nominating", err, "assignment_required_before_approve")
}

func TestControlsAreEmptyOutsideProgress(t *testing.T) {
	e := pf14Setup(t)
	draft, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
	controls := pf15Controls(t, e, "sales-rep", draft)
	if controls.CancelApproval || controls.Raise || controls.AddCustomStep || len(controls.RemovableCustomStepIDs) != 0 {
		t.Fatalf("draft controls=%+v, want none", controls)
	}
}
