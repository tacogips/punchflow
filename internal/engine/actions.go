package engine

import (
	"context"
	"strings"
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
func pf14ValidateActionComment(ctx context.Context, tx *store.Tx, t model.Ticket, comment *string, required bool) error {
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return err
	}
	hasComment := comment != nil && strings.TrimSpace(*comment) != ""
	if hasComment && !def.CommentingEnabled {
		return kferr.ValidationFailed(map[string][]string{"comment": {"comments are disabled"}})
	}
	if !hasComment && required {
		return kferr.ValidationFailed(map[string][]string{"comment": {"is required"}})
	}
	return nil
}
func pf14RequireApproveActor(ctx context.Context, tx *store.Tx, a Actor, t model.Ticket, active []model.TicketStep, now time.Time) error {
	hasApproveStep := false
	for _, step := range active {
		if step.ActionType != model.ActionTypeApprove {
			continue
		}
		hasApproveStep = true
		assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return err
		}
		for _, assignee := range assignees {
			if assignee.Completed {
				continue
			}
			if assignee.UserID == a.UserID {
				return nil
			}
			proxy, err := store.EngActiveProxyApprovers(ctx, tx, assignee.UserID, a.UserID, t.WorkflowID, kftime.FormatDate(now))
			if err != nil {
				return err
			}
			if proxy {
				return nil
			}
		}
	}
	if !hasApproveStep {
		return kferr.InvalidState("Current step does not support rejection or denial")
	}
	return kferr.UserNotAssigned()
}
func pf14CheckNextCustomApprovers(ctx context.Context, tx *store.Tx, t model.Ticket) error {
	if t.RouteID == nil {
		return nil
	}
	steps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	nextOrder := 0
	for _, step := range steps {
		if step.StepOrder > t.CurrentStep && !pf15StepDone(step) && (nextOrder == 0 || step.StepOrder < nextOrder) {
			nextOrder = step.StepOrder
		}
	}
	if nextOrder == 0 {
		return nil
	}
	route, err := store.RouteDefByID(ctx, tx, *t.RouteID)
	if err != nil {
		return err
	}
	// Ticket step orders can drift from the route when custom steps are
	// inserted, so the next group is matched to the route by step code.
	nextCodes := map[string]bool{}
	for _, step := range steps {
		if step.StepOrder == nextOrder && step.RouteStepCode != nil {
			nextCodes[*step.RouteStepCode] = true
		}
	}
	for _, routeStep := range route.Steps {
		if !nextCodes[routeStep.Code] || routeStep.StepType != model.StepTypeAssigneeCustomizable {
			continue
		}
		approvers, err := store.EngCustomApprovers(ctx, tx, t.ID, routeStep.Code)
		if err != nil {
			return err
		}
		if len(approvers) == 0 {
			return kferr.AssignmentRequiredBeforeApprove()
		}
	}
	return nil
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
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	if err = pf14ValidateActionComment(ctx, tx, t, comment, def.CommentingRequiredOnApproval); err != nil {
		return nil, err
	}
	inputs, err := store.EngTicketInputs(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	slips, err := store.TicketSlipItems(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = pf14ValidateApprovalInputs(def, inputs, slips); err != nil {
		return nil, err
	}
	if err = pf14CheckNextCustomApprovers(ctx, tx, t); err != nil {
		return nil, err
	}
	now := e.clock.Now()
	// The webhook event follows the action type of the step the actor acted on,
	// whether or not that step reached its approval threshold (WH-01/PF-02).
	actedApprove, actedConfirm := false, false
	for _, step := range active {
		assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return nil, err
		}
		acted := false
		for i := range assignees {
			as := &assignees[i]
			if as.Completed {
				continue
			}
			proxy := as.UserID != a.UserID
			if !proxy {
				acted = true
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
					acted = true
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
		if acted {
			if step.ActionType == model.ActionTypeApprove {
				actedApprove = true
			} else {
				actedConfirm = true
			}
		}
		completed := 0
		for _, as := range assignees {
			if as.Completed {
				completed++
			}
		}
		if len(assignees) > 0 && completed >= pf14RequiredApprovals(step, len(assignees)) {
			step.Status = model.StepStatusCompleted
			step.CompletedAt = &now
			step.UpdatedAt = now
			if err = store.EngUpdateStep(ctx, tx, step); err != nil {
				return nil, err
			}
		}
	}
	if err = e.pf14Advance(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{}
	if actedApprove || !actedConfirm {
		events = append(events, pf14Event(model.WebhookEventTicketApproved, id, a.UserID))
	}
	if actedConfirm {
		events = append(events, pf14Event(model.WebhookEventTicketConfirmed, id, a.UserID))
	}
	if t.Status == model.TicketStatusCompleted {
		if err = pf14Number(ctx, tx, &t, def, now, numberingEventCompleted); err != nil {
			return nil, err
		}
		if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
			return nil, err
		}
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

// pf14RequiredApprovals returns how many assignees of a step must act before
// the step is complete: the configured number, else the configured percentage
// rounded up, else every assignee. A step without assignees needs none.
func pf14RequiredApprovals(step model.TicketStep, assignees int) int {
	if assignees == 0 {
		return 0
	}
	required := step.RequiredApprovalsNumber
	if required < 1 {
		if step.RequiredApprovalsPercent > 0 {
			required = (step.RequiredApprovalsPercent*assignees + 99) / 100
			if required < 1 {
				required = 1
			}
		} else {
			required = assignees
		}
	}
	return required
}

func (e *Engine) pf14Advance(ctx context.Context, tx *store.Tx, t *model.Ticket, now time.Time) error {
	steps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	for {
		groupDone := true
		for _, step := range steps {
			if step.StepOrder == t.CurrentStep && !pf15StepDone(step) {
				groupDone = false
			}
		}
		if !groupDone {
			break
		}
		next := 0
		for _, step := range steps {
			if step.StepOrder > t.CurrentStep && !pf15StepDone(step) && (next == 0 || step.StepOrder < next) {
				next = step.StepOrder
			}
		}
		if next == 0 {
			t.Status = model.TicketStatusCompleted
			t.CompletedAt = &now
			t.CurrentStep = 0
			break
		}
		t.CurrentStep = next
		actionable := false
		for i := range steps {
			step := &steps[i]
			if step.StepOrder != next || pf15StepDone(*step) {
				continue
			}
			step.Status = model.StepStatusInProgress
			step.UpdatedAt = now
			if err = e.pf14RefreshTeamAssignments(ctx, tx, t, step, now); err != nil {
				return err
			}
			if pf15StepDone(*step) {
				continue
			}
			assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
			if err != nil {
				return err
			}
			for j := range assignees {
				if assignees[j].AssignedAt == nil {
					assignees[j].AssignedAt = &now
					assignees[j].UpdatedAt = now
					if err = store.EngUpdateAssignee(ctx, tx, assignees[j]); err != nil {
						return err
					}
				}
			}
			if step.ActionType == model.ActionTypeNone && len(assignees) > 0 {
				step.Status = model.StepStatusCompleted
				step.CompletedAt = &now
			} else {
				actionable = true
			}
			if err = store.EngUpdateStep(ctx, tx, *step); err != nil {
				return err
			}
		}
		if actionable {
			break
		}
	}
	t.UpdatedAt = now
	if err = e.pf15SyncSubStatus(ctx, tx, t, now); err != nil {
		return err
	}
	if err = store.EngUpdateTicket(ctx, tx, *t); err != nil {
		return err
	}
	if err = SyncProxySharingForTicket(ctx, tx, t.ID, now); err != nil {
		return err
	}
	return SyncWorkflowManagerSharingForTicket(ctx, tx, t.ID)
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
	if err = pf14RequireApproveActor(ctx, tx, a, t, active, e.clock.Now()); err != nil {
		return nil, err
	}
	if err = pf15CheckSubStatusAllows(ctx, tx, id, subStatusActionReject); err != nil {
		return nil, err
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	if err = pf14ValidateActionComment(ctx, tx, t, comment, def.CommentingRequiredOnRejection); err != nil {
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
	for i := range steps {
		step := &steps[i]
		// Steps skipped by their condition or fallback stay skipped; resetting
		// them to pending would make them wait for approval again.
		if step.StepOrder < to || step.Status == model.StepStatusSkipped {
			continue
		}
		step.Status = model.StepStatusPending
		step.CompletedAt = nil
		step.UpdatedAt = now
		if err = store.EngUpdateStep(ctx, tx, *step); err != nil {
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
	if to == 0 {
		t.Status = model.TicketStatusRejected
		t.CurrentStep = 0
	} else {
		t.CurrentStep = to
		reactivated := false
		for i := range steps {
			step := &steps[i]
			if step.StepOrder != to || step.Status == model.StepStatusSkipped {
				continue
			}
			step.Status = model.StepStatusInProgress
			if err = store.EngUpdateStep(ctx, tx, *step); err != nil {
				return nil, err
			}
			reactivated = true
		}
		if !reactivated {
			// Every step of the target group was skipped: continue with the
			// next group that still needs an approval.
			if err = e.pf14Advance(ctx, tx, &t, now); err != nil {
				return nil, err
			}
		}
	}
	t.UpdatedAt = now
	if err = e.pf15SyncSubStatus(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketRejected, id, a.UserID)}
	if comment != nil && strings.TrimSpace(*comment) != "" {
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
	active, _, err := e.pf14CanAct(ctx, tx, a, t)
	if err != nil {
		return nil, err
	}
	if err = pf14RequireApproveActor(ctx, tx, a, t, active, e.clock.Now()); err != nil {
		return nil, err
	}
	if err = pf15CheckSubStatusAllows(ctx, tx, id, subStatusActionDeny); err != nil {
		return nil, err
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	if err = pf14ValidateActionComment(ctx, tx, t, comment, def.CommentingRequiredOnDenial); err != nil {
		return nil, err
	}
	now := e.clock.Now()
	t.Status = model.TicketStatusDenied
	t.DeniedAt = &now
	t.UpdatedAt = now
	if err = e.pf15SyncSubStatus(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketDenied, id, a.UserID)}
	if comment != nil && strings.TrimSpace(*comment) != "" {
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
	if err = pf15CheckSubStatusAllows(ctx, tx, id, subStatusActionWithdraw); err != nil {
		return nil, err
	}
	t.Status = model.TicketStatusRejected
	t.CurrentStep = 0
	t.UpdatedAt = e.clock.Now()
	if err = e.pf15SyncSubStatus(ctx, tx, &t, t.UpdatedAt); err != nil {
		return nil, err
	}
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
	if err = e.pf15SyncSubStatus(ctx, tx, &t, now); err != nil {
		return nil, err
	}
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketArchived, id, a.UserID)}, nil
}
