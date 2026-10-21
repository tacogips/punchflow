package engine

import (
	"context"
	"errors"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// SyncProxySharingForTicket records the active proxy approvers as viewers and
// watchers for a processing ticket. This snapshot intentionally survives later
// proxy-setting removal; action authorization remains registration based.
func SyncProxySharingForTicket(ctx context.Context, tx *store.Tx, ticketID string, now time.Time) error {
	ticket, err := store.EngLoadTicket(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if ticket.Status != model.TicketStatusInProgress {
		return nil
	}
	steps, err := store.EngTicketSteps(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	date := now.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	for _, step := range steps {
		assignees, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return err
		}
		for _, assignee := range assignees {
			proxies, err := store.ProxyUserIDs(ctx, tx, assignee.UserID, ticket.WorkflowID, date)
			if err != nil {
				return err
			}
			for _, proxyID := range proxies {
				if err := addUserViewer(ctx, tx, ticketID, proxyID); err != nil {
					return err
				}
				if err := store.SetTicketWatchStateIfUnset(ctx, tx, ticketID, proxyID, store.WatchStateWatching, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SyncWorkflowManagerSharingForTicket snapshots configured team-member and
// manager audiences when a ticket is submitted and when a later step becomes
// active. Viewer additions remain after team membership changes.
func SyncWorkflowManagerSharingForTicket(ctx context.Context, tx *store.Tx, ticketID string) error {
	return pf14SyncWorkflowManagerSharingForTicket(ctx, tx, ticketID, false)
}

func pf14SyncWorkflowManagerSharingForTicket(ctx context.Context, tx *store.Tx, ticketID string, allowCompleted bool) error {
	ticket, err := store.EngLoadTicket(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	if ticket.AuthorTeamID == nil {
		return nil
	}
	switch ticket.Status {
	case model.TicketStatusInProgress:
	case model.TicketStatusCompleted:
		if !allowCompleted {
			return nil
		}
	default:
		return nil
	}
	def, err := store.WorkflowDefByID(ctx, tx, ticket.WorkflowID)
	if err != nil {
		return err
	}
	if !def.VisibleToTeamMembers && def.VisibleToManager != "direct" && def.VisibleToManager != "all" {
		return nil
	}
	if def.VisibleToTeamMembers {
		members, err := store.EngActiveTeamMemberIDs(ctx, tx, *ticket.AuthorTeamID)
		if err != nil {
			return err
		}
		for _, userID := range members {
			if err := addUserViewer(ctx, tx, ticketID, userID); err != nil {
				return err
			}
		}
	}
	if def.VisibleToManager != "direct" && def.VisibleToManager != "all" {
		return nil
	}
	teams, err := store.EngTeamAncestors(ctx, tx, *ticket.AuthorTeamID)
	if err != nil {
		return err
	}
	for _, team := range teams {
		leaders, err := store.EngLeadersOfTeam(ctx, tx, team.ID)
		if err != nil {
			return err
		}
		eligible := 0
		for _, userID := range leaders {
			if ticket.AuthorID != nil && userID == *ticket.AuthorID {
				continue
			}
			active, err := store.EngUserActive(ctx, tx, userID)
			if err != nil {
				return err
			}
			if active {
				if err := addUserViewer(ctx, tx, ticketID, userID); err != nil {
					return err
				}
				eligible++
			}
		}
		if def.VisibleToManager == "direct" {
			// Direct manager sharing follows the same nearest eligible manager
			// rule as direct-manager approval: skip the applicant and continue
			// upward until a team contributes at least one active leader.
			if eligible > 0 {
				break
			}
		}
	}
	return nil
}

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
//  5. assignees of any step. Proxy visibility comes from the persisted viewer
//     row created with the ticket's proxy sharing side effect.
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
	if ok, err := ticketReadAllowed(ctx, q, a, t.WorkflowID); err != nil || ok {
		return ok, err
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
	if historicalApprover, err := store.TicketApproverHistory(ctx, q, t.ID, a.UserID); err != nil || historicalApprover {
		return historicalApprover, err
	}
	return pf14AssigneeViewer(ctx, q, a, t)
}

func pf14AssigneeViewer(ctx context.Context, q store.Querier, a Actor, t model.Ticket) (bool, error) {
	steps, err := store.EngTicketSteps(ctx, q, t.ID)
	if err != nil {
		return false, err
	}
	for _, s := range steps {
		assignees, err := store.EngStepAssignees(ctx, q, s.ID)
		if err != nil {
			return false, err
		}
		for _, as := range assignees {
			if as.UserID == a.UserID {
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
	for _, watcher := range def.Watchers {
		if watcher.UserCode != "" {
			id, err := store.UserIDByCode(ctx, q, watcher.UserCode)
			if err == nil && id == a.UserID {
				return true, nil
			}
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return false, err
			}
		}
		if watcher.TeamCode == "" {
			continue
		}
		chart, err := store.ActiveOrgChartID(ctx, q)
		if err != nil {
			return false, err
		}
		team, err := store.TeamIDByCode(ctx, q, chart, watcher.TeamCode)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		if ok, err := pf14UserInTeam(ctx, q, a.UserID, team, false, ""); err != nil || ok {
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
