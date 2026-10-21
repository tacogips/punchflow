// Package access evaluates form-section view restrictions shared by the
// ticket presenter and the engine's input updates.
package access

import (
	"context"
	"errors"
	"fmt"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Caller identifies who is reading or editing a ticket.
type Caller struct {
	UserID string
	// TicketRead is true for administrators holding the ticket_read
	// permission; they see every section (inferred administrator exception).
	TicketRead bool
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
	if s.AllowViewAuthor && t.AuthorID != nil && *t.AuthorID == c.UserID {
		return true, nil
	}
	if s.AllowViewApprover && t.ID != "" {
		ok, err := store.EngIsTicketAssignee(ctx, q, t.ID, c.UserID)
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
