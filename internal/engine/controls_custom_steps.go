package engine

import (
	"context"
	"sort"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

const customStepTitleMaxRunes = 100

// CustomStepInput describes an ad-hoc step added to one ticket (PF-19).
type CustomStepInput struct {
	Title       string   `json:"title"`
	ActionType  string   `json:"actionType"`
	UserIDs     []string `json:"userIds"`
	Instruction *string  `json:"instruction"`
	// AfterStepOrder places the step in a new group right after the group with
	// this order. nil inserts it after the group that is currently waiting.
	AfterStepOrder *int `json:"afterStepOrder"`
}

// AssignCurrentApprover lets a ticket administrator add a distinct active user
// to the current approval step, including one that already has assignments
// (INF-NUM-06).
func (e *Engine) AssignCurrentApprover(ctx context.Context, tx *store.Tx, a Actor, ticketID, ticketStepID, userID string) ([]model.WebhookEvent, error) {
	t, err := e.pf15Guard(ctx, tx, a, ticketID)
	if err != nil {
		return nil, err
	}
	if !a.Permissions[model.PermissionTicketWrite] {
		return nil, kferr.MissingPermission("ticket_write is required")
	}
	if t.Status != model.TicketStatusInProgress {
		return nil, kferr.InvalidState("Ticket is not in progress")
	}
	active, err := pf14bUserActive(ctx, tx, userID)
	if err != nil || !active {
		return nil, kferr.ValidationFailed(map[string][]string{"userId": {"must identify an active user"}})
	}
	steps, err := store.EngTicketSteps(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	var target *model.TicketStep
	for _, step := range steps {
		if step.StepOrder != t.CurrentStep || step.Status != model.StepStatusInProgress {
			continue
		}
		if step.ID == ticketStepID {
			target = &step
		}
		assignees, er := store.EngStepAssignees(ctx, tx, step.ID)
		if er != nil {
			return nil, er
		}
		for _, existing := range assignees {
			if existing.UserID == userID {
				return nil, kferr.ValidationFailed(map[string][]string{"userId": {"is already assigned to the current step"}})
			}
		}
	}
	if target == nil {
		return nil, kferr.InvalidState("The current step is not active")
	}
	now := e.clock.Now()
	assignee := model.TicketAssignee{ID: store.NewID(), TicketStepID: target.ID, TicketID: ticketID, UserID: userID, AssignedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err = store.EngInsertAssignee(ctx, tx, assignee); err != nil {
		return nil, err
	}
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, ticketID, a.UserID)}, nil
}

// pf15InsertPositions lists the step orders a custom step may follow.
func pf15InsertPositions(t model.Ticket, steps []pf15Step) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, step := range steps {
		if step.StepOrder >= t.CurrentStep && !seen[step.StepOrder] {
			seen[step.StepOrder] = true
			out = append(out, step.StepOrder)
		}
	}
	sort.Ints(out)
	return out
}

// pf15CanAddCustomStep applies the eligibility rule (INF-63): the workflow
// enables allowCustomSteps, the ticket is in progress and the caller is the
// applicant or an approver who still has to act on the current group.
func (e *Engine) pf15CanAddCustomStep(ctx context.Context, q store.Querier, a Actor, t model.Ticket, def model.WorkflowDef, steps []pf15Step) error {
	if t.Status != model.TicketStatusInProgress {
		return kferr.InvalidState("Ticket is not in progress")
	}
	if !def.AllowCustomSteps {
		return kferr.InvalidState("Custom steps are not enabled for this workflow")
	}
	if t.AuthorID != nil && *t.AuthorID == a.UserID {
		return nil
	}
	for _, step := range steps {
		if step.StepOrder != t.CurrentStep || step.Status != model.StepStatusInProgress {
			continue
		}
		for _, as := range step.Assignees {
			if as.Completed {
				continue
			}
			acts, err := e.pf15ActsFor(ctx, q, a, t.WorkflowID, as.UserID)
			if err != nil {
				return err
			}
			if acts {
				return nil
			}
		}
	}
	return kferr.MissingPermission("Only the applicant or a current approver can add steps")
}

// pf15CanRemoveCustomStep reports whether the caller may delete the step: its
// author, while nobody has started it.
func pf15CanRemoveCustomStep(a Actor, step pf15Step) bool {
	return step.RouteStepID == nil && step.AuthorID != nil && *step.AuthorID == a.UserID && step.Status == model.StepStatusPending
}

// AddCustomStep inserts an ad-hoc step into the in-progress ticket.
func (e *Engine) AddCustomStep(ctx context.Context, tx *store.Tx, a Actor, id string, in CustomStepInput) ([]model.WebhookEvent, error) {
	t, err := e.pf15Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	steps, err := pf15LoadSteps(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = e.pf15CanAddCustomStep(ctx, tx, a, t, def, steps); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	problems := map[string][]string{}
	switch {
	case title == "":
		problems["title"] = []string{"is required"}
	case len([]rune(title)) > customStepTitleMaxRunes:
		problems["title"] = []string{"is too long"}
	}
	if in.ActionType != model.ActionTypeApprove && in.ActionType != model.ActionTypeConfirm {
		problems["actionType"] = []string{"must be approve or confirm"}
	}
	userIDs, err := pf15ResolveCustomAssignees(ctx, tx, in.UserIDs)
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		problems["userIds"] = []string{"must contain at least one active user"}
	}
	after := t.CurrentStep
	if in.AfterStepOrder != nil {
		after = *in.AfterStepOrder
		valid := false
		for _, position := range pf15InsertPositions(t, steps) {
			valid = valid || position == after
		}
		if !valid {
			problems["afterStepOrder"] = []string{"is not a step the new step can follow"}
		}
	}
	if len(problems) > 0 {
		return nil, kferr.ValidationFailed(problems)
	}
	now := e.clock.Now()
	if err = store.EngShiftStepOrders(ctx, tx, id, after); err != nil {
		return nil, err
	}
	author := a.UserID
	step := model.TicketStep{ID: store.NewID(), TicketID: id, StepOrder: after + 1, Title: &title, ActionType: in.ActionType, RequiredApprovalsPercent: 100, Status: model.StepStatusPending, AuthorID: &author, Instruction: in.Instruction, CreatedAt: now, UpdatedAt: now}
	if err = store.EngInsertStep(ctx, tx, step); err != nil {
		return nil, err
	}
	for _, userID := range userIDs {
		assignee := model.TicketAssignee{ID: store.NewID(), TicketStepID: step.ID, TicketID: id, UserID: userID, CreatedAt: now, UpdatedAt: now}
		if err = store.EngInsertAssignee(ctx, tx, assignee); err != nil {
			return nil, err
		}
	}
	if err = pf15RenumberDisplay(ctx, tx, id); err != nil {
		return nil, err
	}
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, id, a.UserID)}, nil
}

// RemoveCustomStep deletes a custom step its author added and nobody started.
func (e *Engine) RemoveCustomStep(ctx context.Context, tx *store.Tx, a Actor, id, stepID string) ([]model.WebhookEvent, error) {
	t, err := e.pf15Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	if t.Status != model.TicketStatusInProgress {
		return nil, kferr.InvalidState("Ticket is not in progress")
	}
	steps, err := pf15LoadSteps(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var target *pf15Step
	for i := range steps {
		if steps[i].ID == stepID {
			target = &steps[i]
		}
	}
	if target == nil {
		return nil, kferr.ResourceNotFound()
	}
	if target.RouteStepID != nil {
		return nil, kferr.InvalidState("Only custom steps can be removed")
	}
	if !pf15CanRemoveCustomStep(a, *target) {
		return nil, kferr.MissingPermission("Only the author of a pending custom step can remove it")
	}
	if err = store.EngDeleteStep(ctx, tx, stepID); err != nil {
		return nil, err
	}
	if err = store.EngCloseStepOrderGap(ctx, tx, id, target.StepOrder); err != nil {
		return nil, err
	}
	if err = pf15RenumberDisplay(ctx, tx, id); err != nil {
		return nil, err
	}
	t.UpdatedAt = e.clock.Now()
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, id, a.UserID)}, nil
}

// pf15ResolveCustomAssignees returns the distinct active users among ids, in
// the given order. An unknown or inactive user fails validation.
func pf15ResolveCustomAssignees(ctx context.Context, q store.Querier, ids []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		active, err := store.EngUserActive(ctx, q, id)
		if err != nil {
			return nil, err
		}
		if !active {
			return nil, kferr.ValidationFailed(map[string][]string{"userIds": {"contains an unknown or inactive user"}})
		}
		out = append(out, id)
	}
	return out, nil
}

// pf15RenumberDisplay makes displayOrder the 1-based position of each step in
// (stepOrder, previous displayOrder) order, as the route builder numbers them.
func pf15RenumberDisplay(ctx context.Context, q store.Querier, ticketID string) error {
	steps, err := store.EngTicketSteps(ctx, q, ticketID)
	if err != nil {
		return err
	}
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].StepOrder != steps[j].StepOrder {
			return steps[i].StepOrder < steps[j].StepOrder
		}
		return steps[i].DisplayOrder < steps[j].DisplayOrder
	})
	for i, step := range steps {
		if step.DisplayOrder == i+1 {
			continue
		}
		if err = store.EngSetStepDisplayOrder(ctx, q, step.ID, i+1); err != nil {
			return err
		}
	}
	return nil
}
