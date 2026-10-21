package engine

import (
	"context"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// CleanupDeletedUser (INF-79) applies kickflow's user-deletion rules inside the caller's
// transaction: the user's draft, in-progress and rejected tickets are archived,
// and the user's outstanding approver assignments on other in-progress tickets
// are removed. Suspension does neither. It returns a ticket_archived webhook
// event for every archived ticket, attributed to the acting administrator.
func (e *Engine) CleanupDeletedUser(ctx context.Context, tx *store.Tx, actor Actor, userID string) ([]model.WebhookEvent, error) {
	now := e.clock.Now()
	ids, err := store.UserOpenAuthoredTicketIDs(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	events := make([]model.WebhookEvent, 0, len(ids))
	for _, id := range ids {
		ticket, err := store.EngLoadTicket(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		ticket.Status = model.TicketStatusArchived
		ticket.ArchivedAt = &now
		ticket.UpdatedAt = now
		if err = store.EngUpdateTicket(ctx, tx, ticket); err != nil {
			return nil, err
		}
		events = append(events, pf14Event(model.WebhookEventTicketArchived, id, actor.UserID))
	}
	// Archived tickets are no longer in progress, so only assignments on
	// tickets authored by others remain to be removed.
	removed, err := store.DeleteUserOutstandingAssignments(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	reevaluated, err := e.pf12ReevaluateSteps(ctx, tx, actor, removed, now)
	if err != nil {
		return nil, err
	}
	return append(events, reevaluated...), nil
}

// pf12ReevaluateSteps (INF-79) re-applies the engine's progression rules to every step
// that lost an assignment: a step left without assignees takes its fallback
// assignees, or is skipped when the fallback is "skip" (or when no replacement
// can ever exist, as for author-customizable and ad-hoc custom steps), and a
// step whose remaining assignees already satisfy its approval threshold is
// completed. The ticket then advances through the shared pf14Advance routine and
// completes (with numbering and ticket_completed) when nothing is left to do.
func (e *Engine) pf12ReevaluateSteps(ctx context.Context, tx *store.Tx, actor Actor, removed []store.OutstandingAssignment, now time.Time) ([]model.WebhookEvent, error) {
	order := []string{}
	stepsByTicket := map[string]map[string]bool{}
	for _, r := range removed {
		if stepsByTicket[r.TicketID] == nil {
			stepsByTicket[r.TicketID] = map[string]bool{}
			order = append(order, r.TicketID)
		}
		stepsByTicket[r.TicketID][r.StepID] = true
	}
	events := []model.WebhookEvent{}
	for _, ticketID := range order {
		ticketEvents, err := e.pf12ReevaluateTicket(ctx, tx, actor, ticketID, stepsByTicket[ticketID], now)
		if err != nil {
			return nil, err
		}
		events = append(events, ticketEvents...)
	}
	return events, nil
}

func (e *Engine) pf12ReevaluateTicket(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, affected map[string]bool, now time.Time) ([]model.WebhookEvent, error) {
	t, err := store.EngLoadTicket(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.Status != model.TicketStatusInProgress {
		return nil, nil
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	steps, err := store.EngTicketSteps(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	var routeSteps []model.RouteStep
	if t.RouteID != nil {
		route, err := store.RouteDefByID(ctx, tx, *t.RouteID)
		if err != nil {
			return nil, err
		}
		routeSteps = route.Steps
	}
	var values map[string]any
	changed := false
	for i := range steps {
		step := steps[i]
		if !affected[step.ID] || pf15StepDone(step) {
			continue
		}
		assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return nil, err
		}
		if len(assignees) > 0 {
			// The remaining assignees may already satisfy the threshold.
			if step.StepOrder != t.CurrentStep || step.Status != model.StepStatusInProgress {
				continue
			}
			completed := 0
			for _, as := range assignees {
				if as.Completed {
					completed++
				}
			}
			if completed >= pf14RequiredApprovals(step, len(assignees)) {
				step.Status = model.StepStatusCompleted
				step.CompletedAt = &now
				step.UpdatedAt = now
				if err = store.EngUpdateStep(ctx, tx, step); err != nil {
					return nil, err
				}
				changed = true
			}
			continue
		}
		var routeStep *model.RouteStep
		if step.RouteStepCode != nil {
			for j := range routeSteps {
				if routeSteps[j].Code == *step.RouteStepCode {
					routeStep = &routeSteps[j]
					break
				}
			}
		}
		switch {
		case routeStep == nil || routeStep.StepType == model.StepTypeAuthorCustomizable:
			// Nobody can be nominated any more: same as an author-customizable
			// step without nominations at submission.
			step.Status, step.Skip, step.UpdatedAt = model.StepStatusSkipped, true, now
			if err = store.EngUpdateStep(ctx, tx, step); err != nil {
				return nil, err
			}
			changed = true
		case routeStep.FallbackType != nil:
			if values == nil {
				rows, err := store.EngTicketInputs(ctx, tx, ticketID)
				if err != nil {
					return nil, err
				}
				values = pf14InputValues(def, pf14InputsFromRows(def, rows))
			}
			ids, err := pf14bFallbackAssignees(ctx, tx, &t, *routeStep, values)
			if err != nil {
				return nil, err
			}
			for _, uid := range ids {
				as := model.TicketAssignee{ID: store.NewID(), TicketStepID: step.ID, TicketID: ticketID, UserID: uid, CreatedAt: now, UpdatedAt: now}
				if step.Status == model.StepStatusInProgress {
					as.AssignedAt = &now
				}
				if err = store.EngInsertAssignee(ctx, tx, as); err != nil {
					return nil, err
				}
			}
			if len(ids) == 0 {
				result := *routeStep.FallbackType
				step.Fallback, step.FallbackResult, step.UpdatedAt = true, &result, now
				if result == model.FallbackSkip {
					step.Status, step.Skip = model.StepStatusSkipped, true
				}
				if err = store.EngUpdateStep(ctx, tx, step); err != nil {
					return nil, err
				}
			}
			changed = true
		}
		// Without a configured fallback the step keeps waiting: the same
		// definition is rejected at submission, but a deletion cannot be.
	}
	if !changed {
		return nil, nil
	}
	if err = e.pf14Advance(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	if t.Status != model.TicketStatusCompleted {
		return nil, nil
	}
	if err = pf14Number(ctx, tx, &t, def, now, numberingEventCompleted); err != nil {
		return nil, err
	}
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketCompleted, ticketID, actor.UserID)}, nil
}
