// Package access evaluates form-section view restrictions shared by the
// ticket presenter and the engine's input updates.
package access

import (
	"context"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Caller identifies who is reading or editing a ticket.
type Caller struct {
	UserID string
	// TicketRead is true when the caller's ticket_read grant covers this
	// ticket's workflow folder; that scoped grant bypasses section restrictions.
	TicketRead bool
}

// CallerForTicket resolves a principal's ticket_read grant against the
// ticket's workflow folder before it can bypass section-level restrictions.
// INF-SEC-03: folder-scoped ticket_read applies only to tickets in its folders.
func CallerForTicket(ctx context.Context, q store.Querier, principal *auth.Principal, ticket model.Ticket) (Caller, error) {
	caller := Caller{}
	if principal == nil {
		return caller, nil
	}
	caller.UserID = principal.User.ID
	if !principal.Has(model.PermissionTicketRead) {
		return caller, nil
	}
	workflow, err := store.GetWorkflow(ctx, q, ticket.WorkflowID)
	if err != nil {
		return Caller{}, fmt.Errorf("access: resolve ticket workflow folder: %w", err)
	}
	caller.TicketRead = principal.Allows(model.PermissionTicketRead, workflow.FolderID)
	return caller, nil
}

// CallerForTicketID resolves the persisted ticket before calculating its
// resource-aware section caller.
func CallerForTicketID(ctx context.Context, q store.Querier, principal *auth.Principal, ticketID string) (Caller, error) {
	ticket, err := store.EngLoadTicket(ctx, q, ticketID)
	if err != nil {
		return Caller{}, err
	}
	return CallerForTicket(ctx, q, principal, ticket)
}

// SectionViewable reports whether the caller may view, and therefore enter,
// the section of the ticket. Unrestricted sections are always viewable. A
// restricted section is viewable by the author when AllowViewAuthor is set, by
// a current or past approver when AllowViewApprover is set, by designated
// section viewers (user, or team with optional grade and descendants) and by
// ticket_read administrators. The ticket may be a stub (no ID) while it is
// being created.
func SectionViewable(ctx context.Context, q store.Querier, c Caller, t model.Ticket, s model.WorkflowSection) (bool, error) {
	if s.SectionType != model.SectionTypeForm || !s.ViewRestriction || c.TicketRead {
		return true, nil
	}
	if s.AllowViewAuthor && ((t.AuthorID != nil && *t.AuthorID == c.UserID) || (t.ProxyClientUserID != nil && *t.ProxyClientUserID == c.UserID)) {
		return true, nil
	}
	if s.AllowViewApprover && t.ID != "" {
		ok, err := approverOrProxy(ctx, q, t, c.UserID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	for _, viewer := range s.SectionViewers {
		ok, err := viewerMatches(ctx, q, c.UserID, viewer)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// approverOrProxy implements INF-SEC-04's state and active-proxy rules.
func approverOrProxy(ctx context.Context, q store.Querier, ticket model.Ticket, userID string) (bool, error) {
	if ticket.Status != model.TicketStatusInProgress && ticket.Status != model.TicketStatusCompleted {
		return false, nil
	}
	steps, err := store.TicketSteps(ctx, q, ticket.ID)
	if err != nil {
		return false, fmt.Errorf("access: list ticket steps: %w", err)
	}
	for _, step := range steps {
		assignees, err := store.TicketAssignees(ctx, q, step.ID)
		if err != nil {
			return false, fmt.Errorf("access: list ticket assignees: %w", err)
		}
		for _, assignee := range assignees {
			if assignee.UserID == userID || assignee.ActualApproverID != nil && *assignee.ActualApproverID == userID {
				return true, nil
			}
			proxies, err := store.ProxyUserIDs(ctx, q, assignee.UserID, ticket.WorkflowID, time.Now().In(kftime.JST).Format("2006-01-02"))
			if err != nil {
				return false, fmt.Errorf("access: list active approver proxies: %w", err)
			}
			for _, proxyID := range proxies {
				if proxyID == userID {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// SectionAccess returns, for every section of the definition, whether the
// caller may view and enter it.
func SectionAccess(ctx context.Context, q store.Querier, c Caller, t model.Ticket, def model.WorkflowDef) (map[string]bool, error) {
	out := make(map[string]bool, len(def.SectionList))
	for _, s := range def.SectionList {
		ok, err := SectionViewable(ctx, q, c, t, s)
		if err != nil {
			return nil, err
		}
		out[s.ID] = ok
	}
	return out, nil
}

func viewerMatches(ctx context.Context, q store.Querier, userID string, v model.SectionViewer) (bool, error) {
	if v.UserCode != "" {
		id, err := store.UserIDByCode(ctx, q, v.UserCode)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("access: resolve section viewer user: %w", err)
		}
		return id == userID, nil
	}
	if v.TeamCode == "" {
		return false, nil
	}
	chart, err := store.ActiveOrgChartID(ctx, q)
	if err != nil {
		return false, fmt.Errorf("access: active org chart: %w", err)
	}
	teamID, err := store.TeamIDByCode(ctx, q, chart, v.TeamCode)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("access: resolve section viewer team: %w", err)
	}
	teams := map[string]bool{teamID: true}
	if v.Descendants {
		queue := []string{teamID}
		for len(queue) > 0 {
			children, err := store.GetTeamChildren(ctx, q, queue[0])
			if err != nil {
				return false, fmt.Errorf("access: team children: %w", err)
			}
			queue = queue[1:]
			for _, child := range children {
				if !teams[child.ID] {
					teams[child.ID] = true
					queue = append(queue, child.ID)
				}
			}
		}
	}
	gradeID := ""
	if v.GradeCode != "" {
		gradeID, err = store.GradeIDByCode(ctx, q, v.GradeCode)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("access: resolve section viewer grade: %w", err)
		}
	}
	memberships, err := store.EngActiveMemberships(ctx, q, userID, "")
	if err != nil {
		return false, fmt.Errorf("access: memberships: %w", err)
	}
	for _, m := range memberships {
		if !teams[m.TeamID] {
			continue
		}
		if gradeID == "" {
			return true, nil
		}
		for _, g := range m.GradeIDs {
			if g == gradeID {
				return true, nil
			}
		}
	}
	return false, nil
}
