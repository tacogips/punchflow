package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (e *Engine) pf14Create(ctx context.Context, tx *store.Tx, a Actor, in CreateInput) (string, []model.WebhookEvent, error) {
	workflow, err := store.GetWorkflow(ctx, tx, in.WorkflowID)
	if err != nil {
		return "", nil, kferr.ResourceNotFound()
	}
	if workflow.Status != model.WorkflowStatusVisible {
		return "", nil, kferr.ValidationFailed(map[string][]string{"workflowId": {"is unavailable"}})
	}
	def, err := store.WorkflowDefByID(ctx, tx, in.WorkflowID)
	if err != nil {
		return "", nil, err
	}
	if in.Status != model.TicketStatusDraft && in.Status != model.TicketStatusInProgress {
		return "", nil, kferr.InvalidParameter("Invalid ticket status")
	}
	now := e.clock.Now()
	if in.ProxyClientUserID != nil {
		if err = pf14cValidateProxyClient(ctx, tx, a.UserID, *in.ProxyClientUserID, workflow.ID, kftime.FormatDate(now)); err != nil {
			return "", nil, err
		}
	}
	authorTeam, err := store.GetTeam(ctx, tx, in.AuthorTeamID)
	if err != nil {
		return "", nil, kferr.ValidationFailed(map[string][]string{"authorTeamId": {"is invalid"}})
	}
	member, err := store.EngUserIsActiveMember(ctx, tx, a.UserID, authorTeam.ID)
	if err != nil {
		return "", nil, err
	}
	if !member {
		return "", nil, kferr.ValidationFailed(map[string][]string{"authorTeamId": {"is invalid"}})
	}
	if err = pf14CheckApplicable(ctx, tx, a.UserID, def, authorTeam.ID); err != nil {
		return "", nil, err
	}
	full := in.Status == model.TicketStatusInProgress
	inputs, slips := []FieldInput{}, []SlipItemInput{}
	if in.Inputs != nil {
		inputs = *in.Inputs
	}
	if in.SlipItems != nil {
		slips = *in.SlipItems
	}
	authorID := a.UserID
	blocked, err := pf16BlockedSections(ctx, tx, a, model.Ticket{AuthorID: &authorID}, def)
	if err != nil {
		return "", nil, err
	}
	if err = pf16RejectBlockedInputs(def, blocked, inputs); err != nil {
		return "", nil, err
	}
	if err = pf14ValidateCreateCompleteness(def, inputs, blocked); err != nil {
		return "", nil, err
	}
	id := store.NewID()
	form, err := pf14NormalizeForm(ctx, tx, id, def, inputs, slips, formOpts{Full: full, SlipMode: slipReplaceAll, Blocked: blocked}, store.Millis(now))
	if err != nil {
		return "", nil, err
	}
	if form.Title != nil {
		in.Title = form.Title
	}
	inputs = pf14cInputsFromRows(def, form.Inputs)
	snapshotName, err := store.TeamFullName(ctx, tx, authorTeam.ID)
	if err != nil {
		return "", nil, err
	}
	snapshot, err := json.Marshal(map[string]string{"id": authorTeam.ID, "name": authorTeam.Name, "fullName": snapshotName, "code": authorTeam.Code})
	if err != nil {
		return "", nil, err
	}
	author := a.UserID
	t := model.Ticket{ID: id, WorkflowID: workflow.ID, Status: in.Status, AuthorID: &author, AuthorTeamID: &authorTeam.ID, AuthorTeamSnapshot: snapshot, ProxyClientUserID: in.ProxyClientUserID, Title: in.Title, ForcedPublicType: model.ForcedPublicTypeFollowWorkflow, CreatedAt: now, UpdatedAt: now}
	if err = pf14Number(ctx, tx, &t, def, now, numberingEventCreated); err != nil {
		return "", nil, err
	}
	if err = store.EngInsertTicket(ctx, tx, t); err != nil {
		return "", nil, err
	}
	if err = store.EngReplaceInputs(ctx, tx, id, form.Inputs); err != nil {
		return "", nil, err
	}
	if err = store.EngReplaceSlipItems(ctx, tx, id, form.Slips); err != nil {
		return "", nil, err
	}
	if err = pf14SyncInputSideEffects(ctx, tx, def, id, nil, nil, form, store.Millis(now)); err != nil {
		return "", nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketCreated, id, a.UserID)}
	if !full {
		if err = pf14SaveAuthorNominations(ctx, tx, id, def, in.Approvers); err != nil {
			return "", nil, err
		}
		return id, events, nil
	}
	if err = e.pf14BuildRoute(ctx, tx, &t, def, inputs, in.Approvers, store.Millis(now)); err != nil {
		return "", nil, err
	}
	submitted, err := pf14FinishSubmission(ctx, tx, &t, def, a.UserID, now)
	if err != nil {
		return "", nil, err
	}
	return id, append(events, submitted...), nil
}

func pf14cValidateProxyClient(ctx context.Context, q store.Querier, actorID, clientID, workflowID, date string) error {
	registered, err := store.EngActiveProxyApplicant(ctx, q, clientID, actorID, workflowID, date)
	if err != nil {
		return fmt.Errorf("engine: validate proxy client registration: %w", err)
	}
	active, err := store.EngUserActive(ctx, q, clientID)
	if err != nil {
		return fmt.Errorf("engine: validate proxy client status: %w", err)
	}
	if !registered || !active {
		return kferr.ValidationFailed(map[string][]string{"proxyClientUserId": {"is invalid"}})
	}
	return nil
}

// pf14ValidateCreateCompleteness requires every form field to be present,
// except those in sections the actor cannot enter.
func pf14ValidateCreateCompleteness(def model.WorkflowDef, inputs []FieldInput, blocked map[string]bool) error {
	for _, section := range def.SectionList {
		if section.SectionType != model.SectionTypeForm || blocked[section.ID] {
			continue
		}
		for _, field := range section.FormFields {
			found := false
			for _, input := range inputs {
				if input.FormFieldID != "" && input.FormFieldID == field.ID || input.FormFieldID == "" && input.FormFieldCode == field.Code {
					found = true
					break
				}
			}
			if !found {
				return kferr.InvalidParameter("Missing form field: " + field.Code)
			}
		}
	}
	return nil
}
