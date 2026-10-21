package engine

import (
	"context"
	"errors"
	"sort"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf15Step is a ticket step together with its assignees.
type pf15Step struct {
	model.TicketStep
	Assignees []model.TicketAssignee
}

// TicketControls reports which runtime lifecycle controls (PF-19) the caller
// may use on a ticket right now. It mirrors the checks of the operations.
type TicketControls struct {
	CancelApproval         bool     `json:"cancelApproval"`
	Raise                  bool     `json:"raise"`
	RaiseTargetStepOrders  []int    `json:"raiseTargetStepOrders"`
	AddCustomStep          bool     `json:"addCustomStep"`
	CustomStepInsertAfter  []int    `json:"customStepInsertAfter"`
	RemovableCustomStepIDs []string `json:"removableCustomStepIds"`
}

// pf15StepDone reports whether a step no longer needs any action.
func pf15StepDone(step model.TicketStep) bool {
	switch step.Status {
	case model.StepStatusCompleted, model.StepStatusSkipped, model.StepStatusRaised:
		return true
	}
	return false
}

func pf15LoadSteps(ctx context.Context, q store.Querier, ticketID string) ([]pf15Step, error) {
	steps, err := store.EngTicketSteps(ctx, q, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]pf15Step, 0, len(steps))
	for _, step := range steps {
		assignees, err := store.EngStepAssignees(ctx, q, step.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, pf15Step{TicketStep: step, Assignees: assignees})
	}
	return out, nil
}

// pf15ActsFor reports whether the actor may act as the assignee: either the
// assignee themself or a user holding an active proxy approval for them.
func (e *Engine) pf15ActsFor(ctx context.Context, q store.Querier, a Actor, workflowID, assigneeID string) (bool, error) {
	if assigneeID == a.UserID {
		return true, nil
	}
	return store.EngActiveProxyApprovers(ctx, q, assigneeID, a.UserID, workflowID, kftime.FormatDate(e.clock.Now()))
}

// pf15Guard loads the ticket and applies the visibility and tenant-mode checks
// shared by every ticket operation.
func (e *Engine) pf15Guard(ctx context.Context, q store.Querier, a Actor, id string) (model.Ticket, error) {
	return e.pf14Guard(ctx, q, a, id)
}

// Controls reports the runtime controls available to the caller.
func (e *Engine) Controls(ctx context.Context, q store.Querier, a Actor, id string) (TicketControls, error) {
	out := TicketControls{RaiseTargetStepOrders: []int{}, CustomStepInsertAfter: []int{}, RemovableCustomStepIDs: []string{}}
	t, err := e.pf15Guard(ctx, q, a, id)
	if err != nil {
		return out, err
	}
	if t.Status != model.TicketStatusInProgress {
		return out, nil
	}
	def, err := store.WorkflowDefByID(ctx, q, t.WorkflowID)
	if err != nil {
		return out, err
	}
	steps, err := pf15LoadSteps(ctx, q, id)
	if err != nil {
		return out, err
	}
	if _, err = e.pf15CancelTarget(ctx, q, a, t, def, steps); err == nil {
		out.CancelApproval = true
	} else if _, domain := kferr.As(err); !domain {
		return out, err
	}
	targets, err := e.pf15RaiseTargets(ctx, q, a, t, steps)
	if err != nil {
		return out, err
	}
	out.RaiseTargetStepOrders = append(out.RaiseTargetStepOrders, targets...)
	out.Raise = len(targets) > 0
	if err = e.pf15CanAddCustomStep(ctx, q, a, t, def, steps); err == nil {
		out.AddCustomStep = true
		out.CustomStepInsertAfter, err = pf15EligibleInsertPositions(ctx, q, t, steps)
		if err != nil {
			return out, err
		}
	} else if _, domain := kferr.As(err); !domain {
		return out, err
	}
	for _, step := range steps {
		if pf15CanRemoveCustomStep(a, step) {
			out.RemovableCustomStepIDs = append(out.RemovableCustomStepIDs, step.ID)
		}
	}
	return out, nil
}

// pf15CancelTarget finds the step order whose approval the actor may cancel.
// Rule (INF-62): the actor is assigned to a completed step (directly or through
// an active proxy relationship), and the ticket is in that group or one group
// later; approvals in the immediately following group prohibit cancellation.
func (e *Engine) pf15CancelTarget(ctx context.Context, q store.Querier, a Actor, t model.Ticket, def model.WorkflowDef, steps []pf15Step) (int, error) {
	if t.Status != model.TicketStatusInProgress {
		return 0, kferr.InvalidState("Ticket is not in progress")
	}
	if !def.ApprovalCancellable {
		return 0, kferr.InvalidState("Approval cancellation is not enabled for this workflow")
	}
	target := -1
	for _, step := range steps {
		for _, as := range step.Assignees {
			if !as.Completed {
				continue
			}
			acts, err := e.pf15ActsFor(ctx, q, a, t.WorkflowID, as.UserID)
			if err != nil {
				return 0, err
			}
			if acts && step.StepOrder <= t.CurrentStep && t.CurrentStep-step.StepOrder <= 1 && step.StepOrder > target {
				target = step.StepOrder
			}
		}
	}
	if target < 0 {
		return 0, kferr.InvalidState("The caller has no approval to cancel")
	}
	for _, step := range steps {
		if step.StepOrder != target+1 {
			continue
		}
		for _, as := range step.Assignees {
			if as.Completed {
				return 0, kferr.InvalidState("A later step has already been acted on")
			}
		}
	}
	return target, nil
}

// CancelApproval withdraws the caller's latest approval so the step becomes
// current again and the caller may act on it anew.
func (e *Engine) CancelApproval(ctx context.Context, tx *store.Tx, a Actor, id string, comment *string) ([]model.WebhookEvent, error) {
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
	target, err := e.pf15CancelTarget(ctx, tx, a, t, def, steps)
	if err != nil {
		return nil, err
	}
	now := e.clock.Now()
	for _, step := range steps {
		switch {
		case step.StepOrder == target:
			reopened := false
			for _, as := range step.Assignees {
				if as.Completed {
					acts, actErr := e.pf15ActsFor(ctx, tx, a, t.WorkflowID, as.UserID)
					if actErr != nil {
						return nil, actErr
					}
					if !acts {
						continue
					}
					as.Completed, as.CompletedAt, as.ActualApproverID, as.UpdatedAt = false, nil, nil, now
					if err = store.EngUpdateAssignee(ctx, tx, as); err != nil {
						return nil, err
					}
					reopened = true
				}
			}
			if reopened && step.Status != model.StepStatusInProgress {
				step.Status, step.CompletedAt, step.UpdatedAt = model.StepStatusInProgress, nil, now
				if err = store.EngUpdateStep(ctx, tx, step.TicketStep); err != nil {
					return nil, err
				}
			}
		case step.StepOrder > target && (step.Status == model.StepStatusInProgress || step.Status == model.StepStatusCompleted):
			// Later steps that had become current, or that completed on their
			// own because they need no action, wait again.
			step.Status, step.CompletedAt, step.UpdatedAt = model.StepStatusPending, nil, now
			if err = store.EngUpdateStep(ctx, tx, step.TicketStep); err != nil {
				return nil, err
			}
			for _, as := range step.Assignees {
				as.AssignedAt, as.UpdatedAt = nil, now
				if err = store.EngUpdateAssignee(ctx, tx, as); err != nil {
					return nil, err
				}
			}
		}
	}
	t.CurrentStep, t.CompletedAt, t.UpdatedAt = target, nil, now
	if err = e.pf15SyncSubStatus(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, id, a.UserID)}
	if comment != nil && strings.TrimSpace(*comment) != "" {
		_, commentEvents, commentErr := e.pf14AddComment(ctx, tx, a, id, *comment, nil)
		if commentErr != nil {
			return nil, commentErr
		}
		events = append(events, commentEvents...)
	}
	return events, nil
}

// pf15RaiseTargets lists later groups that authorize raising: the actor is an
// assignee (or active proxy) and at least one route step permits raising.
func (e *Engine) pf15RaiseTargets(ctx context.Context, q store.Querier, a Actor, t model.Ticket, steps []pf15Step) ([]int, error) {
	if t.Status != model.TicketStatusInProgress || t.RouteID == nil {
		return []int{}, nil
	}
	route, err := store.RouteDefByID(ctx, q, *t.RouteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return []int{}, nil
		}
		return nil, err
	}
	allow := map[string]bool{}
	for _, routeStep := range route.Steps {
		allow[routeStep.Code] = routeStep.AllowRaising
	}
	seen := map[int]bool{}
	targets := []int{}
	for _, step := range steps {
		if step.StepOrder <= t.CurrentStep || step.Status != model.StepStatusPending || step.RouteStepCode == nil || !allow[*step.RouteStepCode] || seen[step.StepOrder] {
			continue
		}
		for _, as := range step.Assignees {
			if as.Completed {
				continue
			}
			acts, err := e.pf15ActsFor(ctx, q, a, t.WorkflowID, as.UserID)
			if err != nil {
				return nil, err
			}
			if acts {
				seen[step.StepOrder] = true
				targets = append(targets, step.StepOrder)
				break
			}
		}
	}
	sort.Ints(targets)
	return targets, nil
}

// Raise pulls the ticket up to a later step the caller is assigned to. The
// steps in between are marked raised and the target group becomes current.
// to selects the target step order; nil picks the nearest eligible one.
func (e *Engine) Raise(ctx context.Context, tx *store.Tx, a Actor, id string, to *int) ([]model.WebhookEvent, error) {
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
	targets, err := e.pf15RaiseTargets(ctx, tx, a, t, steps)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, kferr.InvalidState("The caller cannot raise this ticket")
	}
	if to != nil {
		target := *to
		found := false
		for _, candidate := range targets {
			found = found || candidate == target
		}
		if !found {
			return nil, kferr.ValidationFailed(map[string][]string{"to": {"is not a step the caller can raise to"}})
		}
	}
	now := e.clock.Now()
	for _, step := range steps {
		if step.StepOrder != t.CurrentStep || pf15StepDone(step.TicketStep) {
			continue
		}
		step.Status, step.UpdatedAt = model.StepStatusRaised, now
		if err = store.EngUpdateStep(ctx, tx, step.TicketStep); err != nil {
			return nil, err
		}
	}
	// The authorization target may be later, but one operation only raises the
	// current group and activates the next unfinished group.
	if err = e.pf14Advance(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{}, nil
}
