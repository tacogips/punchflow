package engine

import (
	"context"
	"errors"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14EffectiveVisibility is the single place that decides whether an actor may
// read a ticket (INF-36). Detail reads, ticket lists and file access all go
// through Engine.CanView, which delegates here. Rules, in order:
//
//  1. the author, the proxy client and holders of ticket_read/ticket_write;
//  2. workflow-level viewers (team members, configured users and teams);
//  3. everyone when the ticket is public (forced public, or following a public
//     workflow), except while it is still a draft;
//  4. ticket viewers added through POST /v1/tickets/{id}/viewers (user, or team
//     with optional grade and descendants), except while it is still a draft;
//  5. assignees of any step and their active proxy approvers.
func (e *Engine) pf14EffectiveVisibility(ctx context.Context, q store.Querier, a Actor, t model.Ticket) (bool, error) {
	if t.AuthorID != nil && *t.AuthorID == a.UserID {
		return true, nil
	}
	if t.ProxyClientUserID != nil && *t.ProxyClientUserID == a.UserID {
		return true, nil
	}
	if a.Permissions[model.PermissionTicketRead] || a.Permissions[model.PermissionTicketWrite] {
		return true, nil
	}
	def, err := store.WorkflowDefByID(ctx, q, t.WorkflowID)
	if err != nil {
		return false, err
	}
	if allowed, err := pf14WorkflowViewer(ctx, q, a, t, def); err != nil || allowed {
		return allowed, err
	}
	if t.Status != model.TicketStatusDraft {
		// Drafts are never public and are not shared through ticket viewers.
		if model.TicketIsPublic(t.ForcedPublicType, def.PublicTicket) {
			return true, nil
		}
		if allowed, err := pf14TicketViewer(ctx, q, a, t.ID); err != nil || allowed {
			return allowed, err
		}
	}
	return e.pf14AssigneeViewer(ctx, q, a, t)
}

func (e *Engine) pf14AssigneeViewer(ctx context.Context, q store.Querier, a Actor, t model.Ticket) (bool, error) {
	steps, err := store.EngTicketSteps(ctx, q, t.ID)
	if err != nil {
		return false, err
	}
	date := kftime.FormatDate(e.clock.Now())
	for _, s := range steps {
		assignees, err := store.EngStepAssignees(ctx, q, s.ID)
		if err != nil {
			return false, err
		}
		for _, as := range assignees {
			if as.UserID == a.UserID {
				return true, nil
			}
			ok, err := store.EngActiveProxyApprovers(ctx, q, as.UserID, a.UserID, t.WorkflowID, date)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// pf14TicketViewer reports whether a persisted ticket viewer row grants access.
func pf14TicketViewer(ctx context.Context, q store.Querier, a Actor, ticketID string) (bool, error) {
	viewers, err := store.TicketViewers(ctx, q, ticketID)
	if err != nil {
		return false, err
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == a.UserID {
			return true, nil
		}
		if viewer.TeamID == nil {
			continue
		}
		gradeID := ""
		if viewer.GradeID != nil {
			gradeID = *viewer.GradeID
		}
		ok, err := pf14UserInTeam(ctx, q, a.UserID, *viewer.TeamID, viewer.Descendants, gradeID)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// pf14UserInTeam reports whether the user has an active membership in the team
// (or, with descendants, in any team below it), optionally holding the grade.
func pf14UserInTeam(ctx context.Context, q store.Querier, userID, teamID string, descendants bool, gradeID string) (bool, error) {
	teamIDs, err := pf14bTargetTeamIDs(ctx, q, teamID, descendants)
	if err != nil {
		return false, err
	}
	for _, id := range teamIDs {
		memberships, err := store.EngActiveMemberships(ctx, q, userID, id)
		if err != nil {
			return false, err
		}
		for _, membership := range memberships {
			if gradeID == "" || pf14Contains(membership.GradeIDs, gradeID) {
				return true, nil
			}
		}
	}
	return false, nil
}

func pf14WorkflowViewer(ctx context.Context, q store.Querier, a Actor, ticket model.Ticket, def model.WorkflowDef) (bool, error) {
	if ticket.AuthorTeamID != nil && def.VisibleToTeamMembers {
		ok, err := store.EngUserIsActiveMember(ctx, q, a.UserID, *ticket.AuthorTeamID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	for _, viewer := range def.TicketViewers {
		if viewer.UserCode != "" {
			id, err := store.UserIDByCode(ctx, q, viewer.UserCode)
			if err == nil && id == a.UserID {
				return true, nil
			}
		}
		if viewer.TeamCode == "" {
			continue
		}
		chart, err := store.ActiveOrgChartID(ctx, q)
		if err != nil {
			return false, err
		}
		team, err := store.TeamIDByCode(ctx, q, chart, viewer.TeamCode)
		if err != nil {
			continue
		}
		gradeID := ""
		if viewer.GradeCode != "" {
			if gradeID, err = store.GradeIDByCode(ctx, q, viewer.GradeCode); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				return false, err
			}
		}
		ok, err := pf14UserInTeam(ctx, q, a.UserID, team, viewer.Descendants, gradeID)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (e *Engine) pf14CanView(ctx context.Context, q store.Querier, a Actor, id string) error {
	t, err := store.EngLoadTicket(ctx, q, id)
	if err != nil {
		return kferr.ResourceNotFound()
	}
	visible, err := e.pf14EffectiveVisibility(ctx, q, a, t)
	if err != nil {
		return err
	}
	if !visible {
		return kferr.InvisibleTicket()
	}
	return nil
}
