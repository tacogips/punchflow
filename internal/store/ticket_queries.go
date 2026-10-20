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
			parts = append(parts, "EXISTS (SELECT 1 FROM proxy_approvers pa WHERE pa.user_id="+alias+".user_id AND pa.proxy_user_id=? AND (pa.starts_on IS NULL OR pa.starts_on<=?) AND (pa.ends_on IS NULL OR pa.ends_on>=?) AND (pa.workflow_ids='[]' OR EXISTS (SELECT 1 FROM json_each(pa.workflow_ids) workflow WHERE workflow.value=t.workflow_id)))")
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
		add("EXISTS (SELECT 1 FROM teams tm WHERE tm.id=t.author_team_id AND tm.full_name=?)", filter.AuthorTeamFullName)
	}
	if filter.TicketNumber != "" {
		add("t.ticket_number=?", filter.TicketNumber)
	}
	if filter.Tasks {
		assigneeWhere, values := taskAssigneeWhere("ta")
		values = append(values, model.StepStatusInProgress)
		add("EXISTS (SELECT 1 FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id WHERE ta.ticket_id=t.id AND "+assigneeWhere+" AND ta.completed=0 AND ts.status=?)", values...)
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
		clause := "EXISTS (SELECT 1 FROM ticket_assignees ta WHERE ta.ticket_id=t.id AND ta.user_id=?"
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
						parts = append(parts, "ta.completed=0")
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
		add("0=1")
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
		add("EXISTS (SELECT 1 FROM ticket_steps ts WHERE ts.ticket_id=t.id AND ts.title=?)", filter.StepTitle)
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

// TicketQueryViewers returns a page of viewer IDs in the API's stable order.
func TicketQueryViewers(ctx context.Context, q Querier, ticketID string, offset, limit int, sortByCreatedAtDesc bool) ([]string, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM ticket_viewers WHERE ticket_id=?`, ticketID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count ticket viewers: %w", err)
	}
	order := `coalesce(tm.name, u.email), tv.id`
	if sortByCreatedAtDesc {
		order = `tv.rowid DESC`
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
