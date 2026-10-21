package present

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

type TicketSummaryJSON map[string]any
type TicketJSON map[string]any
type TicketWithStepJSON map[string]any

func TicketSummary(ctx context.Context, q store.Querier, id string) (TicketSummaryJSON, error) {
	x, err := store.Ticket(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("present ticket summary: %w", err)
	}
	wf, err := Workflow(ctx, q, x.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("present ticket workflow: %w", err)
	}
	comments, logs, links, err := store.TicketCounts(ctx, q, id)
	if err != nil {
		return nil, err
	}
	// Tickets stored before creation set a value default to following the workflow.
	if x.ForcedPublicType == "" {
		x.ForcedPublicType = model.ForcedPublicTypeFollowWorkflow
	}
	public := model.TicketIsPublic(x.ForcedPublicType, wf["publicTicket"] == true)
	var snapshot any
	if len(x.AuthorTeamSnapshot) != 0 {
		if err = json.Unmarshal(x.AuthorTeamSnapshot, &snapshot); err != nil {
			return nil, fmt.Errorf("present author team snapshot: %w", err)
		}
	}
	var workflow any = wf
	return TicketSummaryJSON{"id": x.ID, "ticketNumber": nullableString(x.TicketNumber), "title": nullableString(x.Title), "status": x.Status, "currentStep": x.CurrentStep, "createdAt": pf08Time(x.CreatedAt), "openedAt": nullableTimeValue(x.OpenedAt), "completedAt": nullableTimeValue(x.CompletedAt), "archivedAt": nullableTimeValue(x.ArchivedAt), "updatedAt": pf08Time(x.UpdatedAt), "publicStatus": public, "forcedPublicType": x.ForcedPublicType, "workflow": workflow, "authorTeamId": nullableString(x.AuthorTeamID), "authorTeamSnapshot": snapshot, "commentsCount": comments, "logsCount": logs, "linkedTicketsCount": links, "closedAt": nullableTimeValue(x.ClosedAt), "deniedAt": nullableTimeValue(x.DeniedAt), "useRouteStepCondition": true}, nil
}

func Ticket(ctx context.Context, q store.Querier, id string) (TicketJSON, error) {
	summary, err := TicketSummary(ctx, q, id)
	if err != nil {
		return nil, err
	}
	x, err := store.Ticket(ctx, q, id)
	if err != nil {
		return nil, err
	}
	var author, team, proxy any
	if x.AuthorID != nil {
		author, err = User(ctx, q, *x.AuthorID, UserOpts{})
		if err != nil {
			return nil, err
		}
	}
	if x.AuthorTeamID != nil {
		team, err = Team(ctx, q, *x.AuthorTeamID)
		if err != nil {
			return nil, err
		}
	}
	if x.ProxyClientUserID != nil {
		proxy, err = User(ctx, q, *x.ProxyClientUserID, UserOpts{})
		if err != nil {
			return nil, err
		}
	}
	out := TicketJSON(summary)
	out["author"] = author
	out["authorTeam"] = team
	out["proxyClientUser"] = proxy
	out["subStatus"] = nil
	out["labels"] = []any{}
	return out, nil
}

func Tickets(ctx context.Context, q store.Querier, ids []string) ([]TicketJSON, error) {
	out := make([]TicketJSON, 0, len(ids))
	for _, id := range ids {
		x, e := Ticket(ctx, q, id)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, nil
}

func TicketWithStep(ctx context.Context, q store.Querier, id string) (TicketWithStepJSON, error) {
	ticket, err := Ticket(ctx, q, id)
	if err != nil {
		return nil, err
	}
	steps, err := ticketStepJSON(ctx, q, id, false)
	if err != nil {
		return nil, err
	}
	out := TicketWithStepJSON(ticket)
	out["steps"] = steps
	out["ticketGuest"] = nil
	out["triggerTicket"] = nil
	return out, nil
}

func TicketsWithStep(ctx context.Context, q store.Querier, ids []string) ([]TicketWithStepJSON, error) {
	out := make([]TicketWithStepJSON, 0, len(ids))
	for _, id := range ids {
		x, e := TicketWithStep(ctx, q, id)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, nil
}

func ProxyUserIDs(ctx context.Context, q store.Querier, userID, workflowID string, today string) ([]string, error) {
	return store.ProxyUserIDs(ctx, q, userID, workflowID, today)
}

func ticketStepJSON(ctx context.Context, q store.Querier, ticketID string, detail bool) ([]any, error) {
	steps, err := store.TicketSteps(ctx, q, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(steps))
	ticket, err := store.Ticket(ctx, q, ticketID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		assignees, err := store.TicketAssignees(ctx, q, step.ID)
		if err != nil {
			return nil, err
		}
		assigned := make([]any, 0, len(assignees))
		for _, a := range assignees {
			user, err := User(ctx, q, a.UserID, UserOpts{})
			if err != nil {
				return nil, err
			}
			current := ticket.Status == model.TicketStatusInProgress && ticket.CurrentStep == step.StepOrder
			proxies := []string{}
			if !a.Completed {
				var e error
				proxies, e = ProxyUserIDs(ctx, q, a.UserID, ticket.WorkflowID, time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"))
				if e != nil {
					return nil, e
				}
			}
			am := map[string]any{"id": a.ID, "stepOrder": step.StepOrder, "current": current, "completed": a.Completed, "completedAt": nullableTimeValue(a.CompletedAt), "pending": false, "user": user, "proxyUserIds": proxies}
			if detail {
				var actual any
				if a.ActualApproverID != nil {
					actual, err = User(ctx, q, *a.ActualApproverID, UserOpts{})
					if err != nil {
						return nil, err
					}
				}
				am["actualApprover"] = actual
			}
			assigned = append(assigned, am)
		}
		m := map[string]any{"id": step.ID, "routeStepId": nullableString(step.RouteStepID), "title": nullableString(step.Title), "actionType": step.ActionType, "requiredApprovalsNumber": step.RequiredApprovalsNumber, "requiredApprovalsPercent": step.RequiredApprovalsPercent, "stepOrder": step.StepOrder, "skip": step.Skip, "fallback": step.Fallback, "fallbackResult": nullableString(step.FallbackResult), "author": nil, "assignees": assigned, "completed": step.Status == model.StepStatusCompleted, "completedAt": nullableTimeValue(step.CompletedAt), "instruction": nil, "displayOrder": step.DisplayOrder, "status": step.Status}
		out = append(out, m)
	}
	return out, nil
}

func nullableString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}
func nullableTimeValue(v *time.Time) any {
	if v == nil {
		return nil
	}
	return pf08Time(*v)
}
