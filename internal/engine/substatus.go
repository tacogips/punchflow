package engine

import (
	"context"
	"errors"
	"sort"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Ticket actions that a sub-status may forbid.
const (
	subStatusActionWithdraw = "withdraw"
	subStatusActionReject   = "reject"
	subStatusActionDeny     = "deny"
)

// pf15SyncSubStatus brings the sub-status attached to the ticket in line with
// its state (INF-60). While the ticket is in progress the sub-status is the one
// configured on the step group that is currently waiting for action; a group
// without a configured sub-status clears it. Every other ticket status carries
// no sub-status. Attaching a sub-status may allocate the ticket number when
// the workflow numbers tickets at sub_status_attached. The caller persists the
// ticket row afterwards, which also stores a newly allocated number.
func (e *Engine) pf15SyncSubStatus(ctx context.Context, tx *store.Tx, t *model.Ticket, now time.Time) error {
	current, err := store.TicketSubStatusID(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	var want *model.SubStatus
	if t.Status == model.TicketStatusInProgress {
		if want, err = pf15ActiveSubStatus(ctx, tx, *t); err != nil {
			return err
		}
	}
	switch {
	case want == nil && current == nil:
		return nil
	case want == nil:
		return store.SetTicketSubStatus(ctx, tx, t.ID, nil)
	case current != nil && *current == want.ID:
		return nil
	}
	if err = store.SetTicketSubStatus(ctx, tx, t.ID, &want.ID); err != nil {
		return err
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return err
	}
	if def.TicketNumberingSubStatus != nil && *def.TicketNumberingSubStatus != want.Code {
		return nil
	}
	return pf14Number(ctx, tx, t, def, now, numberingEventSubStatusAttached)
}

// pf15ActiveSubStatus resolves the sub-status of the step group the ticket is
// waiting on, or nil when no step of the group configures one.
func pf15ActiveSubStatus(ctx context.Context, tx *store.Tx, t model.Ticket) (*model.SubStatus, error) {
	if t.RouteID == nil {
		return nil, nil
	}
	steps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return nil, err
	}
	route, err := store.RouteDefByID(ctx, tx, *t.RouteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	configured := map[string]*string{}
	for _, routeStep := range route.Steps {
		configured[routeStep.Code] = routeStep.SubStatus
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].DisplayOrder < steps[j].DisplayOrder })
	for _, step := range steps {
		if step.StepOrder != t.CurrentStep || step.Status != model.StepStatusInProgress || step.RouteStepCode == nil {
			continue
		}
		code := configured[*step.RouteStepCode]
		if code == nil || *code == "" {
			continue
		}
		item, err := store.SubStatusByCode(ctx, tx, *code)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return &item, nil
	}
	return nil, nil
}

// pf15CheckSubStatusAllows refuses an action that the ticket's current
// sub-status forbids. API callers and UI users hit the same check (INF-61).
func pf15CheckSubStatusAllows(ctx context.Context, q store.Querier, ticketID, action string) error {
	id, err := store.TicketSubStatusID(ctx, q, ticketID)
	if err != nil || id == nil {
		return err
	}
	item, err := store.SubStatusByID(ctx, q, *id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	switch action {
	case subStatusActionWithdraw:
		if !item.AllowWithdrawal {
			return kferr.InvalidState("Withdrawal is not allowed while the ticket has the sub-status " + item.Name)
		}
	case subStatusActionReject:
		if !item.AllowRejection {
			return kferr.InvalidState("Sending back is not allowed while the ticket has the sub-status " + item.Name)
		}
	case subStatusActionDeny:
		if !item.AllowDenial {
			return kferr.InvalidState("Denial is not allowed while the ticket has the sub-status " + item.Name)
		}
	}
	return nil
}
