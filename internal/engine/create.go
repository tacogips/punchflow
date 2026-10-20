package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"punchflow/internal/kferr"
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
	if !def.AvailableToEveryone && len(def.AvailableTeams) > 0 {
		allowed := false
		for _, team := range def.AvailableTeams {
			id, er := store.TeamIDByCode(ctx, tx, mustActiveChart(ctx, tx), team.TeamCode)
			if er == nil {
				ok, er := store.EngUserIsActiveMember(ctx, tx, a.UserID, id)
				if er != nil {
					return "", nil, er
				}
				allowed = allowed || ok
			}
		}
		if !allowed {
			return "", nil, kferr.TicketCreateRestricted()
		}
	}
	full := in.Status == model.TicketStatusInProgress
	inputs, slips := []FieldInput{}, []SlipItemInput{}
	if in.Inputs != nil {
		inputs = *in.Inputs
	}
	if in.SlipItems != nil {
		slips = *in.SlipItems
	}
	id := store.NewID()
	normalized, normalizedSlips, err := pf14NormalizeForm(ctx, tx, id, def, inputs, slips, full, store.Millis(now))
	if err != nil {
		return "", nil, err
	}
	snapshotName, err := store.TeamFullName(ctx, tx, authorTeam.ID)
	if err != nil {
		return "", nil, err
	}
	snapshot, err := json.Marshal(map[string]string{"id": authorTeam.ID, "name": authorTeam.Name, "fullName": snapshotName, "code": authorTeam.Code})
	if err != nil {
		return "", nil, err
	}
	author := a.UserID
	t := model.Ticket{ID: id, WorkflowID: workflow.ID, Status: in.Status, AuthorID: &author, AuthorTeamID: &authorTeam.ID, AuthorTeamSnapshot: snapshot, ProxyClientUserID: in.ProxyClientUserID, Title: in.Title, CreatedAt: now, UpdatedAt: now}
	if err = store.EngInsertTicket(ctx, tx, t); err != nil {
		return "", nil, err
	}
	if err = store.EngReplaceInputs(ctx, tx, id, normalized); err != nil {
		return "", nil, err
	}
	if err = store.EngReplaceSlipItems(ctx, tx, id, normalizedSlips); err != nil {
		return "", nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketCreated, id, a.UserID)}
	if full {
		if err = e.pf14BuildRoute(ctx, tx, &t, def, inputs, in.Approvers, store.Millis(now)); err != nil {
			return "", nil, err
		}
		if def.TicketNumberingTiming == "opened" {
			if err = pf14Number(ctx, tx, &t, def, now); err != nil {
				return "", nil, err
			}
		}
		t.OpenedAt = &now
		t.UpdatedAt = now
		if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
			return "", nil, err
		}
		events = append(events, pf14Event(model.WebhookEventTicketOpened, id, a.UserID))
	}
	return id, events, nil
}

func mustActiveChart(ctx context.Context, tx *store.Tx) string {
	id, err := store.ActiveOrgChartID(ctx, tx)
	if err != nil {
		return ""
	}
	return id
}
func pf14ErrorContext(err error, what string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("engine: %s: %w", what, err)
}
