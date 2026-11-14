package engine

import (
	"context"
	"encoding/json"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// NominateNextApprovers lets the current approver choose assignees for the
// next assignee_customizable route step through the local UI API.
func (e *Engine) NominateNextApprovers(ctx context.Context, tx *store.Tx, actor Actor, ticketID, routeStepCode string, userIDs []string) ([]model.WebhookEvent, error) {
	ticket, err := e.pf15Guard(ctx, tx, actor, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.Status != model.TicketStatusInProgress || ticket.RouteID == nil {
		return nil, kferr.InvalidState("Ticket has no active approval route")
	}
	steps, err := store.EngTicketSteps(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	actorCanNominate := false
	for _, step := range steps {
		if step.StepOrder != ticket.CurrentStep || step.Status != model.StepStatusInProgress {
			continue
		}
		assignees, loadErr := store.EngStepAssignees(ctx, tx, step.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		for _, assignee := range assignees {
			if assignee.Completed {
				continue
			}
			acts := assignee.UserID == actor.UserID
			if !acts {
				acts, err = store.EngActiveProxyApprovers(ctx, tx, assignee.UserID, actor.UserID, ticket.WorkflowID, kftime.FormatDate(e.clock.Now()))
				if err != nil {
					return nil, err
				}
			}
			if acts {
				actorCanNominate = true
				break
			}
		}
	}
	if !actorCanNominate {
		return nil, kferr.UserNotAssigned()
	}
	route, err := store.RouteDefByID(ctx, tx, *ticket.RouteID)
	if err != nil {
		return nil, err
	}
	routeByCode := make(map[string]model.RouteStep, len(route.Steps))
	for _, routeStep := range route.Steps {
		routeByCode[routeStep.Code] = routeStep
	}
	nextOrder := 0
	for _, step := range steps {
		if step.StepOrder <= ticket.CurrentStep || pf15StepDone(step) {
			continue
		}
		if nextOrder == 0 || step.StepOrder < nextOrder {
			nextOrder = step.StepOrder
		}
	}
	var target *model.RouteStep
	var targetTicketSteps []model.TicketStep
	for _, step := range steps {
		if step.StepOrder != nextOrder || step.RouteStepCode == nil || pf15StepDone(step) {
			continue
		}
		routeStep, ok := routeByCode[*step.RouteStepCode]
		if ok && routeStep.Code == routeStepCode && routeStep.StepType == model.StepTypeAssigneeCustomizable {
			target = &routeStep
			targetTicketSteps = append(targetTicketSteps, step)
		}
	}
	if target == nil || target.Code != routeStepCode {
		return nil, kferr.InvalidState("Nomination target is not the next customizable step")
	}
	uniqueIDs := pf14UniqueUserIDs(userIDs)
	minimum := 1
	if target.MinCustomAssignees != nil {
		minimum = *target.MinCustomAssignees
	}
	if len(uniqueIDs) < minimum {
		return nil, kferr.ValidationFailed(map[string][]string{"userIds": {"does not meet the minimum nominee count"}})
	}
	if len(uniqueIDs) == 0 && minimum != 0 {
		return nil, kferr.ValidationFailed(map[string][]string{"userIds": {"at least one nominee is required"}})
	}
	inputs, err := store.EngTicketInputs(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	fieldCodes := map[string]string{}
	definition, err := store.WorkflowDefByID(ctx, tx, ticket.WorkflowID)
	if err != nil {
		return nil, err
	}
	for _, section := range definition.SectionList {
		for _, field := range section.FormFields {
			fieldCodes[field.ID] = field.Code
		}
	}
	fieldInputs := make([]FieldInput, 0, len(inputs))
	for _, input := range inputs {
		raw, marshalErr := json.Marshal(input.Value)
		if marshalErr != nil {
			return nil, marshalErr
		}
		fieldInputs = append(fieldInputs, FieldInput{FormFieldID: input.FormFieldID, FormFieldCode: fieldCodes[input.FormFieldID], Value: raw})
	}
	values := pf14InputValues(definition, fieldInputs)
	for _, userID := range uniqueIDs {
		if err = pf14bValidateNominee(ctx, tx, userID, target.Targets, values); err != nil {
			return nil, err
		}
	}
	if err = store.EngInsertCustomApprovers(ctx, tx, ticketID, target.Code, uniqueIDs); err != nil {
		return nil, err
	}
	now := e.clock.Now()
	for _, step := range targetTicketSteps {
		assignees, loadErr := store.EngStepAssignees(ctx, tx, step.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		for _, assignee := range assignees {
			if _, err = tx.ExecContext(ctx, `DELETE FROM ticket_assignees WHERE id=?`, assignee.ID); err != nil {
				return nil, err
			}
		}
		if len(uniqueIDs) == 0 {
			step.Status, step.Skip, step.UpdatedAt = model.StepStatusSkipped, true, now
			if err = store.EngUpdateStep(ctx, tx, step); err != nil {
				return nil, err
			}
			continue
		}
		for _, userID := range uniqueIDs {
			assignee := model.TicketAssignee{ID: store.NewID(), TicketStepID: step.ID, TicketID: ticketID, UserID: userID, CreatedAt: now, UpdatedAt: now}
			if err = store.EngInsertAssignee(ctx, tx, assignee); err != nil {
				return nil, err
			}
		}
	}
	ticket.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, ticket); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, ticketID, actor.UserID)}, nil
}
