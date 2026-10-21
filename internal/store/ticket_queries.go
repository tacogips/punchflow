package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"punchflow/internal/model"
)

// TicketQueryFilter contains the documented ticket and task list filters.
type TicketQueryFilter struct {
	UserID, WorkflowID, AuthorID, AuthorTeamFullName, TicketNumber string
	TaskUserIDs                                                    []string
	TaskProxyUserID, TaskProxyDate                                 string
	VisibleTicketIDs                                               []string
	AssigneeUserID, StepTitle                                      string
	Statuses, AssigneeStatuses                                     []string
	SubStatusIDs                                                   []string
	CreatedAtStart, CreatedAtEnd                                   *time.Time
	UpdatedAtStart, UpdatedAtEnd                                   *time.Time
	OpenedAtStart, OpenedAtEnd                                     *time.Time
	CompletedAtStart, CompletedAtEnd                               *time.Time
	ArchivedAtStart, ArchivedAtEnd                                 *time.Time
	AssignedToMeAtStart, AssignedToMeAtEnd                         *time.Time
	SortBy                                                         string
	Desc                                                           bool
	Tasks                                                          bool
}

// TicketQueryList returns matching ticket IDs and a total before pagination.
func TicketQueryList(ctx context.Context, q Querier, filter TicketQueryFilter, offset, limit int) ([]string, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, values ...any) { where = append(where, clause); args = append(args, values...) }
	taskAssigneeWhere := func(alias string) (string, []any) {
		users := filter.TaskUserIDs
		if len(users) == 0 {
			users = []string{filter.UserID}
		}
		marks := make([]string, len(users))
		values := make([]any, 0, len(users)+3)
		for i, userID := range users {
			marks[i] = "?"
			values = append(values, userID)
		}
		parts := []string{alias + ".user_id IN (" + strings.Join(marks, ",") + ")"}
		if filter.TaskProxyUserID != "" {
			parts = append(parts, "EXISTS (SELECT 1 FROM proxy_approvers pa JOIN users principal ON principal.id=pa.user_id JOIN users proxy ON proxy.id=pa.proxy_user_id WHERE pa.user_id="+alias+".user_id AND pa.proxy_user_id=? AND principal.status='activated' AND principal.deactivated_at IS NULL AND proxy.status='activated' AND proxy.deactivated_at IS NULL AND (pa.starts_on IS NULL OR pa.starts_on<=?) AND (pa.ends_on IS NULL OR pa.ends_on>=?) AND (pa.workflow_ids='[]' OR EXISTS (SELECT 1 FROM json_each(pa.workflow_ids) workflow WHERE workflow.value=t.workflow_id)))")
			values = append(values, filter.TaskProxyUserID, filter.TaskProxyDate, filter.TaskProxyDate)
		}
		return "(" + strings.Join(parts, " OR ") + ")", values
	}
	if filter.VisibleTicketIDs != nil {
		ids, err := json.Marshal(filter.VisibleTicketIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("store: encode visible ticket IDs: %w", err)
		}
		add("t.id IN (SELECT value FROM json_each(?))", string(ids))
	}
	if filter.WorkflowID != "" {
		add("t.workflow_id=?", filter.WorkflowID)
	}
	if filter.AuthorID != "" {
		add("t.author_id=?", filter.AuthorID)
	}
	if filter.AuthorTeamFullName != "" {
		add(`EXISTS (
			WITH RECURSIVE ancestors(id,parent_id,name,depth) AS (
				SELECT id,parent_id,name,0 FROM teams WHERE id=t.author_team_id
				UNION ALL
				SELECT parent.id,parent.parent_id,parent.name,ancestors.depth+1
				FROM teams parent JOIN ancestors ON parent.id=ancestors.parent_id
			)
			SELECT 1 FROM (SELECT group_concat(name,'/') AS full_name FROM (SELECT name FROM ancestors ORDER BY depth DESC))
			WHERE full_name=?
		)`, filter.AuthorTeamFullName)
	}
	if filter.TicketNumber != "" {
		add("t.ticket_number=?", filter.TicketNumber)
	}
	if filter.Tasks {
		assigneeWhere, values := taskAssigneeWhere("ta")
		// No explicit status means actionable outstanding assignments. Explicit
		// historical statuses retain the caller's completed assignment rows.
		if len(filter.Statuses) == 0 {
			values = append(values, model.StepStatusInProgress)
			add("EXISTS (SELECT 1 FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id WHERE ta.ticket_id=t.id AND "+assigneeWhere+" AND ta.completed=0 AND ts.status=?)", values...)
		} else {
			add("EXISTS (SELECT 1 FROM ticket_assignees ta WHERE ta.ticket_id=t.id AND "+assigneeWhere+")", values...)
		}
		if filter.AssignedToMeAtStart != nil {
			assigneeWhere, values = taskAssigneeWhere("ta")
			values = append(values, Millis(*filter.AssignedToMeAtStart))
			add("EXISTS (SELECT 1 FROM ticket_assignees ta WHERE ta.ticket_id=t.id AND "+assigneeWhere+" AND ta.assigned_at>=?)", values...)
		}
		if filter.AssignedToMeAtEnd != nil {
			assigneeWhere, values = taskAssigneeWhere("ta")
			values = append(values, Millis(*filter.AssignedToMeAtEnd))
			add("EXISTS (SELECT 1 FROM ticket_assignees ta WHERE ta.ticket_id=t.id AND "+assigneeWhere+" AND ta.assigned_at<=?)", values...)
		}
	} else if filter.AssigneeUserID != "" {
		clause := "EXISTS (SELECT 1 FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id WHERE ta.ticket_id=t.id AND ta.user_id=?"
		values := []any{filter.AssigneeUserID}
		if len(filter.AssigneeStatuses) > 0 {
			states := []string{}
			for _, state := range filter.AssigneeStatuses {
				switch state {
				case "all":
					states = append(states, "all")
				case "current":
					states = append(states, "current")
				case "approved":
					states = append(states, "approved")
				}
			}
			if len(states) > 0 {
				parts := []string{}
				for _, state := range states {
					switch state {
					case "all":
						parts = append(parts, "1=1")
					case "current":
						parts = append(parts, "ta.completed=0 AND ts.status='in_progress' AND ts.step_order=t.current_step")
					case "approved":
						parts = append(parts, "ta.completed=1")
					}
				}
				clause += " AND (" + strings.Join(parts, " OR ") + ")"
			}
		}
		add(clause+")", values...)
	}
	if len(filter.SubStatusIDs) > 0 {
		marks := make([]string, len(filter.SubStatusIDs))
		values := make([]any, len(filter.SubStatusIDs))
		for i, id := range filter.SubStatusIDs {
			marks[i], values[i] = "?", id
		}
		add("t.sub_status_id IN ("+strings.Join(marks, ",")+")", values...)
	}
	if len(filter.Statuses) > 0 {
		marks := make([]string, len(filter.Statuses))
		for i, value := range filter.Statuses {
			marks[i] = "?"
			args = append(args, value)
		}
		where = append(where, "t.status IN ("+strings.Join(marks, ",")+")")
	}
	for _, pair := range []struct {
		column     string
		start, end *time.Time
	}{
		{"t.created_at", filter.CreatedAtStart, filter.CreatedAtEnd}, {"t.updated_at", filter.UpdatedAtStart, filter.UpdatedAtEnd},
		{"t.opened_at", filter.OpenedAtStart, filter.OpenedAtEnd}, {"t.completed_at", filter.CompletedAtStart, filter.CompletedAtEnd}, {"t.archived_at", filter.ArchivedAtStart, filter.ArchivedAtEnd},
	} {
		if pair.start != nil {
			add(pair.column+">=?", Millis(*pair.start))
		}
		if pair.end != nil {
			add(pair.column+"<=?", Millis(*pair.end))
		}
	}
	if filter.StepTitle != "" {
		add("EXISTS (SELECT 1 FROM ticket_steps ts WHERE ts.ticket_id=t.id AND ts.title=? AND ts.status='in_progress' AND ts.step_order=t.current_step)", filter.StepTitle)
	}
	order := "t.created_at DESC"
	orderArgs := []any{}
	if filter.SortBy != "" {
		columns := map[string]string{"createdAt": "t.created_at", "updatedAt": "t.updated_at", "openedAt": "t.opened_at"}
		if column, ok := columns[filter.SortBy]; ok {
			direction := " ASC"
			if filter.Desc {
				direction = " DESC"
			}
			order = column + direction + ", t.id" + direction
		} else if filter.SortBy == "assignedToMeAt" {
			assigneeWhere, values := taskAssigneeWhere("ta")
			direction := " ASC"
			if filter.Desc {
				direction = " DESC"
			}
			order = "(SELECT min(ta.assigned_at) FROM ticket_assignees ta WHERE ta.ticket_id=t.id AND " + assigneeWhere + ")" + direction + ", t.id" + direction
			orderArgs = append(orderArgs, values...)
		}
	}
	base := " FROM tickets t WHERE " + strings.Join(where, " AND ")
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*)"+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count ticket query: %w", err)
	}
	queryArgs := append(append([]any{}, args...), orderArgs...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := q.QueryContext(ctx, "SELECT t.id"+base+" ORDER BY "+order+" LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list ticket query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan ticket query: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate ticket query: %w", err)
	}
	return ids, total, nil
}

// TicketQueryLinks returns linked tickets for the requested ticket.
func TicketQueryLinks(ctx context.Context, q Querier, ticketID string, offset, limit int) ([]string, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM ticket_links WHERE ticket_id=?`, ticketID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count ticket links: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT linked_ticket_id FROM ticket_links WHERE ticket_id=? ORDER BY created_at,linked_ticket_id LIMIT ? OFFSET ?`, ticketID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list ticket links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

// TicketQueryInsertViewer inserts one ticket viewer.
func TicketQueryInsertViewer(ctx context.Context, q Querier, v model.TicketViewer) error {
	_, err := q.ExecContext(ctx, `INSERT INTO ticket_viewers(id,ticket_id,user_id,team_id,grade_id,descendants) VALUES(?,?,?,?,?,?)`, v.ID, v.TicketID, v.UserID, v.TeamID, v.GradeID, v.Descendants)
	if err != nil {
		return fmt.Errorf("store: insert ticket viewer: %w", err)
	}
	return nil
}

// TicketQueryDeleteViewer removes the identified viewer from a ticket.
func TicketQueryDeleteViewer(ctx context.Context, q Querier, ticketID, viewerID string) error {
	result, err := q.ExecContext(ctx, `DELETE FROM ticket_viewers WHERE ticket_id=? AND id=?`, ticketID, viewerID)
	if err != nil {
		return fmt.Errorf("store: delete ticket viewer: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// TicketQueryViewers returns a page of viewer IDs. When byCreatedAt is false
// the default order (team name, then user email) applies; otherwise viewers are
// ordered by creation time ascending or, when desc is set, descending.
func TicketQueryViewers(ctx context.Context, q Querier, ticketID string, offset, limit int, byCreatedAt, desc bool) ([]string, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM ticket_viewers WHERE ticket_id=?`, ticketID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count ticket viewers: %w", err)
	}
	order := `coalesce(tm.name, u.email), tv.id`
	if byCreatedAt {
		order = `tv.rowid ASC`
		if desc {
			order = `tv.rowid DESC`
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT tv.id FROM ticket_viewers tv LEFT JOIN teams tm ON tm.id=tv.team_id LEFT JOIN users u ON u.id=tv.user_id WHERE tv.ticket_id=? ORDER BY `+order+` LIMIT ? OFFSET ?`, ticketID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list ticket viewers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan ticket viewer: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate ticket viewers: %w", err)
	}
	return ids, total, nil
}
