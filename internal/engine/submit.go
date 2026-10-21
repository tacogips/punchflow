package engine

import (
	"context"
	"errors"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14FinishSubmission completes the submission of a ticket whose route has
// just been built and advanced by pf14BuildRoute. It records openedAt, applies
// the "opened" numbering timing and, when the route finished on its own (every
// group skipped or needing no user action), the "completed" timing. It persists
// the ticket and returns the lifecycle events in emission order: ticket_opened,
// then ticket_completed if the ticket is already complete.
func pf14FinishSubmission(ctx context.Context, tx *store.Tx, t *model.Ticket, def model.WorkflowDef, actorID string, now time.Time) ([]model.WebhookEvent, error) {
	if err := pf14Number(ctx, tx, t, def, now, numberingEventOpened); err != nil {
		return nil, err
	}
	if t.OpenedAt == nil {
		t.OpenedAt = &now
	}
	completed := t.Status == model.TicketStatusCompleted
	if completed {
		if err := pf14Number(ctx, tx, t, def, now, numberingEventCompleted); err != nil {
			return nil, err
		}
	}
	t.UpdatedAt = now
	if err := store.EngUpdateTicket(ctx, tx, *t); err != nil {
		return nil, err
	}
	if err := SyncProxySharingForTicket(ctx, tx, t.ID, now); err != nil {
		return nil, err
	}
	if err := SyncWorkflowManagerSharingForTicket(ctx, tx, t.ID); err != nil {
		return nil, err
	}
	events := []model.WebhookEvent{pf14Event(model.WebhookEventTicketOpened, t.ID, actorID)}
	if completed {
		events = append(events, pf14Event(model.WebhookEventTicketCompleted, t.ID, actorID))
	}
	return events, nil
}

// pf14CheckApplicable enforces the workflow's application restriction
// (INF-28). A workflow that is available to everyone, or that lists no team,
// is open. Otherwise the ticket's author team must be one of the listed teams
// (or, when the entry sets descendants, a team below it) and the user must hold
// a grade accepted by the entry's grade restriction in the author team.
func pf14CheckApplicable(ctx context.Context, q store.Querier, userID string, def model.WorkflowDef, authorTeamID string) error {
	if def.AvailableToEveryone || len(def.AvailableTeams) == 0 {
		return nil
	}
	chart, err := store.ActiveOrgChartID(ctx, q)
	if err != nil {
		return err
	}
	ancestors, err := store.EngTeamAncestors(ctx, q, authorTeamID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	for _, available := range def.AvailableTeams {
		listed, err := store.TeamIDByCode(ctx, q, chart, available.TeamCode)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		for depth, ancestor := range ancestors {
			if ancestor.ID != listed || (depth > 0 && !available.Descendants) {
				continue
			}
			ok, err := pf14bUserMatchesGrade(ctx, q, userID, authorTeamID, available.GradeSymbol, available.GradeCodes)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
	return kferr.TicketCreateRestricted()
}

// pf14SaveAuthorNominations validates and persists applicant-nominated
// approvers for author_customizable route steps of a draft or rejected ticket.
// The route is not materialized before submission, so the step codes are
// checked against the route the ticket's stored inputs currently select; when
// that route cannot be determined yet the check is deferred to submission.
// Saving replaces the nomination of each named step.
func pf14SaveAuthorNominations(ctx context.Context, tx *store.Tx, ticketID string, def model.WorkflowDef, approvers []ApproverInput) error {
	if len(approvers) == 0 {
		return nil
	}
	var route *model.RouteDef
	rows, err := store.EngTicketInputs(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	values := pf14InputValues(def, pf14InputsFromRows(def, rows))
	if routeCode, selectErr := pf14SelectRouteCode(def, values); selectErr == nil {
		if routeID, idErr := store.RouteIDByCode(ctx, tx, routeCode); idErr == nil {
			if selected, defErr := store.RouteDefByID(ctx, tx, routeID); defErr == nil {
				route = &selected
			}
		}
	}
	for _, nomination := range approvers {
		if nomination.RouteStepCode == "" {
			return kferr.InvalidParameter("Invalid customizable route step")
		}
		if route != nil {
			valid := false
			for _, step := range route.Steps {
				if step.Code == nomination.RouteStepCode && step.StepType == model.StepTypeAuthorCustomizable {
					valid = true
					break
				}
			}
			if !valid {
				return kferr.InvalidParameter("Invalid customizable route step")
			}
		}
		userIDs, err := pf14ResolveUserRefs(ctx, tx, nomination.UserIDs)
		if err != nil {
			return err
		}
		if len(userIDs) != len(nomination.UserIDs) {
			return kferr.ValidationFailed(map[string][]string{"approvers": {"contains an unknown user"}})
		}
		if err = store.EngInsertCustomApprovers(ctx, tx, ticketID, nomination.RouteStepCode, userIDs); err != nil {
			return err
		}
	}
	return nil
}
