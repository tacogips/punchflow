package engine

import (
	"context"
	"fmt"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (e *Engine) pf14Guard(ctx context.Context, q store.Querier, a Actor, id string) (model.Ticket, error) {
	t, err := store.EngLoadTicket(ctx, q, id)
	if err != nil {
		return t, kferr.ResourceNotFound()
	}
	if err = e.pf14CanView(ctx, q, a, id); err != nil {
		return t, err
	}
	mode, err := store.EngTenantMode(ctx, q)
	if err != nil {
		return t, err
	}
	if mode == model.TenantModeAdminWorking {
		return t, kferr.AdminIsWorking()
	}
	return t, nil
}
func (e *Engine) pf14CanView(ctx context.Context, q store.Querier, a Actor, id string) error {
	t, err := store.EngLoadTicket(ctx, q, id)
	if err != nil {
		return kferr.ResourceNotFound()
	}
	if t.AuthorID != nil && *t.AuthorID == a.UserID {
		return nil
	}
	if a.Permissions[model.PermissionTicketRead] {
		return nil
	}
	def, err := store.WorkflowDefByID(ctx, q, t.WorkflowID)
	if err != nil {
		return err
	}
	if allowed, err := pf14WorkflowViewer(ctx, q, a, t, def); err != nil {
		return err
	} else if allowed {
		return nil
	}
	steps, err := store.EngTicketSteps(ctx, q, id)
	if err != nil {
		return err
	}
	for _, s := range steps {
		assignees, err := store.EngStepAssignees(ctx, q, s.ID)
		if err != nil {
			return err
		}
		for _, as := range assignees {
			if as.UserID == a.UserID {
				return nil
			}
			ok, err := store.EngActiveProxyApprovers(ctx, q, as.UserID, a.UserID, t.WorkflowID, kftime.FormatDate(e.clock.Now()))
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
	return kferr.InvisibleTicket()
}
func (e *Engine) pf14CanAct(ctx context.Context, tx *store.Tx, a Actor, t model.Ticket) ([]model.TicketStep, []int, error) {
	if t.Status != model.TicketStatusInProgress {
		return nil, nil, kferr.InvalidState("Ticket is not in progress")
	}
	steps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return nil, nil, err
	}
	active := []model.TicketStep{}
	indices := []int{}
	for i, s := range steps {
		if s.StepOrder == t.CurrentStep && s.Status == model.StepStatusInProgress {
			active = append(active, s)
			indices = append(indices, i)
		}
	}
	if len(active) == 0 {
		return nil, nil, kferr.InvalidState("No active approval step")
	}
	acting := false
	for _, s := range active {
		assignees, err := store.EngStepAssignees(ctx, tx, s.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, as := range assignees {
			if as.Completed {
				continue
			}
			if as.UserID == a.UserID {
				acting = true
				continue
			}
			ok, err := store.EngActiveProxyApprovers(ctx, tx, as.UserID, a.UserID, t.WorkflowID, kftime.FormatDate(e.clock.Now()))
			if err != nil {
				return nil, nil, err
			}
			acting = acting || ok
		}
	}
	if !acting {
		return nil, nil, kferr.UserNotAssigned()
	}
	return active, indices, nil
}
func pf14Event(kind, id, actor string) model.WebhookEvent {
	return model.WebhookEvent{Type: kind, TicketID: id, ActorUserID: actor}
}
func (e *Engine) pf14Approve(ctx context.Context, tx *store.Tx, a Actor, id string, comment *string) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	active, _, err := e.pf14CanAct(ctx, tx, a, t)
	if err != nil {
		return nil, err
	}
	if comment == nil || *comment == "" {
		def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
		if err != nil {
			return nil, err
		}
		if def.CommentingRequiredOnApproval {
			return nil, kferr.ValidationFailed(map[string][]string{"comment": {"is required"}})
		}
	}
	now := e.clock.Now()
	anyApprove := false
	for _, step := range active {
		assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return nil, err
		}
		for i := range assignees {
			as := &assignees[i]
			if as.Completed {
				continue
			}
			proxy := as.UserID != a.UserID
			if !proxy {
				as.Completed = true
				as.CompletedAt = &now
				as.ActualApproverID = &a.UserID
				as.UpdatedAt = now
				if err = store.EngUpdateAssignee(ctx, tx, *as); err != nil {
					return nil, err
				}
			} else {
				ok, err := store.EngActiveProxyApprovers(ctx, tx, as.UserID, a.UserID, t.WorkflowID, kftime.FormatDate(now))
				if err != nil {
					return nil, err
				}
				if ok {
					as.Completed = true
					as.CompletedAt = &now
					as.ActualApproverID = &a.UserID
					as.UpdatedAt = now
					if err = store.EngUpdateAssignee(ctx, tx, *as); err != nil {
						return nil, err
					}
				}
			}
		}
		completed := 0
		for _, as := range assignees {
			if as.Completed {
				completed++
			}
		}
		required := step.RequiredApprovalsNumber
		if required < 1 {
			if step.RequiredApprovalsPercent > 0 {
				required = (step.RequiredApprovalsPercent*len(assignees) + 99) / 100
				if required < 1 {
					required = 1
				}
			} else {
				required = len(assignees)
			}
		}
		if len(assignees) == 0 {
			required = 0
		}
		if completed >= required {
			step.Status = model.StepStatusCompleted
			step.CompletedAt = &now
			step.UpdatedAt = now
			if err = store.EngUpdateStep(ctx, tx, step); err != nil {
				return nil, err
			}
			if step.ActionType == model.ActionTypeApprove {
				anyApprove = true
			}
		}
	}
	if err = e.pf14Advance(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	kind := model.WebhookEventTicketConfirmed
	if anyApprove {
		kind = model.WebhookEventTicketApproved
	}
	events := []model.WebhookEvent{pf14Event(kind, id, a.UserID)}
	if t.Status == model.TicketStatusCompleted {
		events = append(events, pf14Event(model.WebhookEventTicketCompleted, id, a.UserID))
	}
	if comment != nil && *comment != "" {
		commentID, ce, err := e.pf14AddComment(ctx, tx, a, id, *comment, nil)
		if err != nil {
			return nil, err
		}
		for i := range ce {
			ce[i].CommentID = commentID
		}
		events = append(events, ce...)
	}
	return events, nil
}
func (e *Engine) pf14Advance(ctx context.Context, tx *store.Tx, t *model.Ticket, now time.Time) error {
	steps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	groupDone := true
	for _, s := range steps {
		if s.StepOrder == t.CurrentStep && s.Status != model.StepStatusCompleted && s.Status != model.StepStatusSkipped {
			groupDone = false
		}
	}
	if !groupDone {
		return nil
	}
	next := 0
	for _, s := range steps {
		if s.StepOrder > t.CurrentStep && s.Status != model.StepStatusSkipped {
			if next == 0 || s.StepOrder < next {
				next = s.StepOrder
			}
		}
	}
	if next == 0 {
		t.Status = model.TicketStatusCompleted
		t.CompletedAt = &now
		t.CurrentStep = 0
	} else {
		t.CurrentStep = next
		for _, s := range steps {
			if s.StepOrder == next {
				s.Status = model.StepStatusInProgress
				s.UpdatedAt = now
				if err = store.EngUpdateStep(ctx, tx, s); err != nil {
					return err
				}
			}
		}
	}
	t.UpdatedAt = now
	return store.EngUpdateTicket(ctx, tx, *t)
}
func (e *Engine) pf14SendBack(ctx context.Context, tx *store.Tx, a Actor, id string, to int, comment *string) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	active, _, err := e.pf14CanAct(ctx, tx, a, t)
	if err != nil {
		return nil, err
	}
	if to < 0 || to >= t.CurrentStep {
		return nil, kferr.ValidationFailed(map[string][]string{"to": {"is invalid"}})
	}
	now := e.clock.Now()
	steps, err := store.EngTicketSteps(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.StepOrder >= to {
			step.Status = model.StepStatusPending
			step.CompletedAt = nil
			step.UpdatedAt = now
			if err = store.EngUpdateStep(ctx, tx, step); err != nil {
				return nil, err
			}
			assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
			if err != nil {
				return nil, err
			}
			for _, as := range assignees {
				as.Completed = false
				as.CompletedAt = nil
				as.ActualApproverID = nil
				as.UpdatedAt = now
				if err = store.EngUpdateAssignee(ctx, tx, as); err != nil {
					return nil, err
				}
			}
		}
	}
	if to == 0 {
		t.Status = model.TicketStatusRejected
		t.CurrentStep = 0
	} else {
		t.CurrentStep = to
		for _, step := range steps {
			if step.StepOrder == to {
				step.Status = model.StepStatusInProgress
				if err = store.EngUpdateStep(ctx, tx, step); err != nil {
					return nil, err
				}
			}
		}
	}
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketRejected, id, a.UserID)}
	if comment != nil && *comment != "" {
		cid, ce, err := e.pf14AddComment(ctx, tx, a, id, *comment, nil)
		if err != nil {
			return nil, err
		}
		for i := range ce {
			ce[i].CommentID = cid
		}
		events = append(events, ce...)
	}
	_ = active
	return events, nil
}
func (e *Engine) pf14Deny(ctx context.Context, tx *store.Tx, a Actor, id string, comment *string) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	if _, _, err = e.pf14CanAct(ctx, tx, a, t); err != nil {
		return nil, err
	}
	now := e.clock.Now()
	t.Status = model.TicketStatusDenied
	t.DeniedAt = &now
	t.ClosedAt = &now
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketDenied, id, a.UserID)}
	if comment != nil && *comment != "" {
		cid, ce, err := e.pf14AddComment(ctx, tx, a, id, *comment, nil)
		if err != nil {
			return nil, err
		}
		for i := range ce {
			ce[i].CommentID = cid
		}
		events = append(events, ce...)
	}
	return events, nil
}
func (e *Engine) pf14Withdraw(ctx context.Context, tx *store.Tx, a Actor, id string) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	if t.AuthorID == nil || *t.AuthorID != a.UserID {
		return nil, kferr.MissingPermission("Only the author can withdraw this ticket")
	}
	if t.Status != model.TicketStatusInProgress {
		return nil, kferr.InvalidState("Ticket is not in progress")
	}
	t.Status = model.TicketStatusRejected
	t.CurrentStep = 0
	t.UpdatedAt = e.clock.Now()
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketWithdrawn, id, a.UserID)}, nil
}
func (e *Engine) pf14Archive(ctx context.Context, tx *store.Tx, a Actor, id string) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	if t.Status == model.TicketStatusArchived {
		return nil, kferr.InvalidState("Ticket is already archived")
	}
	if t.Status == model.TicketStatusInProgress || t.Status == model.TicketStatusCompleted || t.Status == model.TicketStatusDenied {
		if !a.Permissions[model.PermissionTicketWrite] {
			return nil, kferr.MissingPermission("Ticket write permission required")
		}
	} else if (t.AuthorID == nil || *t.AuthorID != a.UserID) && !a.Permissions[model.PermissionTicketWrite] {
		return nil, kferr.MissingPermission("Ticket write permission required")
	}
	now := e.clock.Now()
	t.Status = model.TicketStatusArchived
	t.ArchivedAt = &now
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketArchived, id, a.UserID)}, nil
}
func pf14ActionError(message string) error { return fmt.Errorf("engine: %s", message) }
