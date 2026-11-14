package ticketui

import (
	"encoding/json"
	"net/http"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type handoverBody struct {
	FromUserID string   `json:"fromUserId"`
	ToUserID   string   `json:"toUserId"`
	TeamID     string   `json:"teamId"`
	TicketIDs  []string `json:"ticketIds"`
}

type handoverResult struct {
	TicketIDs []string `json:"ticketIds"`
}

func (a *API) handoverApplicants(w http.ResponseWriter, r *http.Request) {
	a.handover(w, r, "applicant")
}

func (a *API) handoverApprovalRequests(w http.ResponseWriter, r *http.Request) {
	a.handover(w, r, "approval")
}

// handover provides the UI-only user lifecycle transfer described by INF-B8-04.
func (a *API) handover(w http.ResponseWriter, r *http.Request, kind string) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, ok := auth.FromContext(r.Context())
	if !ok || !p.Permissions[model.PermissionTicketWrite] {
		kferr.Write(w, kferr.MissingPermission("ticket_write is required"))
		return
	}
	var body handoverBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.FromUserID == "" || body.ToUserID == "" {
		kferr.Write(w, kferr.InvalidParameter("fromUserId and toUserId are required"))
		return
	}
	if len(body.TicketIDs) > 1000 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"ticketIds": {"must contain at most 1000 tickets"}}))
		return
	}
	q := a.d.DB.Q()
	fromID, err := store.ResolveID(r.Context(), q, "users", body.FromUserID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	toID, err := store.ResolveID(r.Context(), q, "users", body.ToUserID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if fromID == toID {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"toUserId": {"must differ from fromUserId"}}))
		return
	}
	toUser, err := store.GetUser(r.Context(), q, toID)
	if err != nil || toUser.Status != model.UserStatusActive {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"toUserId": {"must identify an active user"}}))
		return
	}
	var targetTeam *model.Team
	if kind == "applicant" && body.TeamID != "" {
		teamID, resolveErr := store.ResolveID(r.Context(), q, "teams", body.TeamID)
		if resolveErr != nil {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"teamId": {"must identify an active team"}}))
			return
		}
		team, teamErr := store.GetTeam(r.Context(), q, teamID)
		member, memberErr := store.TeamMembershipExists(r.Context(), q, teamID, toID)
		activeChart, chartErr := store.ActiveOrgChartID(r.Context(), q)
		if teamErr != nil || memberErr != nil || chartErr != nil || !member || team.ApproveOnly || team.OrganizationChartID != activeChart {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"teamId": {"must be an application team for the target user"}}))
			return
		}
		targetTeam = &team
	}
	ids, err := handoverTicketIDs(r, q, body.TicketIDs, kind, fromID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if len(body.TicketIDs) > 0 {
		for _, id := range ids {
			if err = a.d.Engine.CanView(r.Context(), q, actorFromPrincipal(p), id); err != nil {
				kferr.Write(w, kferr.ValidationFailed(map[string][]string{"ticketIds": {"selected tickets must be visible to the administrator"}}))
				return
			}
		}
	}
	changed := []string{}
	events := []model.WebhookEvent{}
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		for _, id := range ids {
			if kind == "applicant" {
				changedTicket, transferErr := handoverApplicantTicket(r, tx, id, fromID, toID, targetTeam, a.d.Clock.Now())
				if transferErr != nil {
					return transferErr
				}
				if changedTicket {
					changed = append(changed, id)
					events = append(events, model.WebhookEvent{Type: model.WebhookEventTicketUpdated, TicketID: id, ActorUserID: p.User.ID})
				}
				continue
			}
			moved, transferErr := handoverApprovalAssignments(r, tx, id, fromID, toID, a.d.Clock.Now())
			if transferErr != nil {
				return transferErr
			}
			if moved {
				changed = append(changed, id)
				events = append(events, model.WebhookEvent{Type: model.WebhookEventTicketUpdated, TicketID: id, ActorUserID: p.User.ID})
			}
		}
		for _, id := range changed {
			uid, ticketID := p.User.ID, id
			action := "ticket.applicant_handover"
			if kind == "approval" {
				action = "ticket.approval_handover"
			}
			if e := store.InsertAuditLog(r.Context(), tx, model.AuditLog{UserID: &uid, Action: action, TicketID: &ticketID, CreatedAt: a.d.Clock.Now()}); e != nil {
				return e
			}
		}
		return a.d.Events.Enqueue(r.Context(), tx, events)
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteJSON(w, http.StatusOK, handoverResult{TicketIDs: changed})
}

func actorFromPrincipal(p *auth.Principal) engine.Actor { return engine.ActorFromPrincipal(p) }

func handoverTicketIDs(r *http.Request, q store.Querier, supplied []string, kind, fromID string) ([]string, error) {
	if len(supplied) > 0 {
		ids := make([]string, 0, len(supplied))
		seen := map[string]bool{}
		for _, suppliedID := range supplied {
			var id string
			err := q.QueryRowContext(r.Context(), `SELECT id FROM tickets WHERE id=?`, suppliedID).Scan(&id)
			if err != nil {
				return nil, kferr.ResourceNotFound()
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		return ids, nil
	}
	var query string
	if kind == "applicant" {
		query = `SELECT id FROM tickets WHERE author_id=? AND status IN ('draft','in_progress','rejected') ORDER BY id`
	} else {
		query = `SELECT DISTINCT t.id FROM tickets t JOIN ticket_assignees a ON a.ticket_id=t.id JOIN ticket_steps s ON s.id=a.ticket_step_id WHERE a.user_id=? AND a.completed=0 AND s.status='in_progress' AND t.status='in_progress' ORDER BY t.id`
	}
	rows, err := q.QueryContext(r.Context(), query, fromID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func handoverApplicantTicket(r *http.Request, tx *store.Tx, id, fromID, toID string, team *model.Team, now time.Time) (bool, error) {
	ticket, err := store.EngLoadTicket(r.Context(), tx, id)
	if err != nil {
		return false, err
	}
	if ticket.AuthorID == nil || *ticket.AuthorID != fromID || (ticket.Status != model.TicketStatusDraft && ticket.Status != model.TicketStatusInProgress && ticket.Status != model.TicketStatusRejected) {
		return false, nil
	}
	if ticket.ProxyClientUserID == nil && team == nil {
		return false, kferr.ValidationFailed(map[string][]string{"teamId": {"is required for applicant tickets without a proxy applicant"}})
	}
	if team != nil {
		teamName, err := store.TeamFullName(r.Context(), tx, team.ID)
		if err != nil {
			return false, err
		}
		snapshot, err := json.Marshal(map[string]string{"id": team.ID, "name": team.Name, "fullName": teamName, "code": team.Code})
		if err != nil {
			return false, err
		}
		ticket.AuthorTeamID = &team.ID
		ticket.AuthorTeamSnapshot = snapshot
	}
	ticket.AuthorID = &toID
	ticket.UpdatedAt = now
	if err = store.EngUpdateTicket(r.Context(), tx, ticket); err != nil {
		return false, err
	}
	return true, nil
}

func handoverApprovalAssignments(r *http.Request, tx *store.Tx, id, fromID, toID string, now time.Time) (bool, error) {
	ticket, err := store.EngLoadTicket(r.Context(), tx, id)
	if err != nil {
		return false, err
	}
	if ticket.Status != model.TicketStatusInProgress {
		return false, nil
	}
	steps, err := store.EngTicketSteps(r.Context(), tx, id)
	if err != nil {
		return false, err
	}
	moved := false
	var route *model.RouteDef
	if ticket.RouteID != nil {
		loaded, routeErr := store.RouteDefByID(r.Context(), tx, *ticket.RouteID)
		if routeErr != nil {
			return false, routeErr
		}
		route = &loaded
	}
	for _, step := range steps {
		if step.Status != model.StepStatusInProgress {
			continue
		}
		assignees, e := store.EngStepAssignees(r.Context(), tx, step.ID)
		if e != nil {
			return false, e
		}
		for _, assignee := range assignees {
			if assignee.UserID != fromID || assignee.Completed {
				continue
			}
			if ticket.AuthorID != nil && *ticket.AuthorID == toID && !routeStepAllowsSelfApproval(route, step.RouteStepCode) {
				return false, kferr.ValidationFailed(map[string][]string{"toUserId": {"handover would create a self-approval assignment"}})
			}
			duplicate := false
			for _, existing := range assignees {
				if existing.ID != assignee.ID && existing.UserID == toID {
					duplicate = true
					break
				}
			}
			if duplicate {
				if _, e = tx.ExecContext(r.Context(), `DELETE FROM ticket_assignees WHERE id=?`, assignee.ID); e != nil {
					return false, e
				}
				moved = true
				continue
			}
			assignee.UserID = toID
			assignee.UpdatedAt = now
			if e = store.EngUpdateAssignee(r.Context(), tx, assignee); e != nil {
				return false, e
			}
			moved = true
		}
	}
	if moved {
		ticket.UpdatedAt = now
		if err = store.EngUpdateTicket(r.Context(), tx, ticket); err != nil {
			return false, err
		}
	}
	return moved, nil
}

func routeStepAllowsSelfApproval(route *model.RouteDef, code *string) bool {
	if route == nil || code == nil {
		return false
	}
	for _, step := range route.Steps {
		if step.Code == *code {
			return step.AllowSelfApproval
		}
	}
	return false
}
