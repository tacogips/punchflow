package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"punchflow/internal/model"
)

// OrgChartList returns a page of organization charts. sortField is "createdAt"
// (the default) or "name"; ties are broken by creation time and ID.
func OrgChartList(ctx context.Context, q Querier, offset, limit int, sortField string, desc bool) ([]model.OrganizationChart, int, error) {
	column := "created_at"
	if sortField == "name" {
		column = "name"
	}
	direction := "ASC"
	if desc {
		direction = "DESC"
	}
	rows, err := q.QueryContext(ctx, "SELECT id FROM organization_charts ORDER BY "+column+" "+direction+",created_at ASC,id ASC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list org charts: %w", err)
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
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]model.OrganizationChart, 0, len(ids))
	for _, id := range ids {
		x, e := GetOrganizationChart(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	n, e := pf08Count(ctx, q, "SELECT count(*) FROM organization_charts")
	return out, n, e
}
func OrgChartInsert(ctx context.Context, q Querier, x model.OrganizationChart) error {
	_, e := q.ExecContext(ctx, "INSERT INTO organization_charts(id,name,current,created_at,updated_at) VALUES(?,?,0,?,?)", x.ID, x.Name, Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if e != nil {
		return fmt.Errorf("store: insert org chart: %w", e)
	}
	return nil
}
func OrgChartUpdate(ctx context.Context, q Querier, id, name string, at time.Time) error {
	res, e := q.ExecContext(ctx, "UPDATE organization_charts SET name=?,updated_at=? WHERE id=?", name, Millis(at), id)
	return pf19RowsAffected(res, e)
}
func OrgChartDelete(ctx context.Context, q Querier, id string) error {
	res, e := q.ExecContext(ctx, "DELETE FROM organization_charts WHERE id=?", id)
	return pf19RowsAffected(res, e)
}
func OrgChartActivate(ctx context.Context, q Querier, id string, at time.Time) error {
	if _, e := q.ExecContext(ctx, "UPDATE organization_charts SET current=0,updated_at=?", Millis(at)); e != nil {
		return e
	}
	res, e := q.ExecContext(ctx, "UPDATE organization_charts SET current=1,updated_at=? WHERE id=?", Millis(at), id)
	return pf19RowsAffected(res, e)
}

// TeamList returns a page of teams of the chart under parentID (root teams when
// nil), optionally limited to the teams admitted by filter.
func TeamList(ctx context.Context, q Querier, chartID string, parentID *string, offset, limit int, filter IDFilter) ([]model.Team, int, error) {
	scope, scopeArgs := filter.clause("id")
	query := `SELECT id FROM teams WHERE organization_chart_id=? AND `
	args := []any{chartID}
	if parentID == nil {
		query += "parent_id IS NULL"
	} else {
		query += "parent_id=?"
		args = append(args, *parentID)
	}
	query += scope + " ORDER BY code,id LIMIT ? OFFSET ?"
	args = append(args, scopeArgs...)
	args = append(args, limit, offset)
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, 0, e
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, 0, e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		return nil, 0, e
	}
	out := make([]model.Team, 0, len(ids))
	for _, id := range ids {
		x, er := GetTeam(ctx, q, id)
		if er != nil {
			return nil, 0, er
		}
		out = append(out, x)
	}
	countQuery := `SELECT count(*) FROM teams WHERE organization_chart_id=? AND `
	countArgs := []any{chartID}
	if parentID == nil {
		countQuery += "parent_id IS NULL"
	} else {
		countQuery += "parent_id=?"
		countArgs = append(countArgs, *parentID)
	}
	countQuery += scope
	countArgs = append(countArgs, scopeArgs...)
	n, er := pf08Count(ctx, q, countQuery, countArgs...)
	return out, n, er
}
func TeamInsert(ctx context.Context, q Querier, x model.Team) error {
	_, e := q.ExecContext(ctx, "INSERT INTO teams(id,organization_chart_id,code,name,notes,parent_id,approve_only,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", x.ID, x.OrganizationChartID, x.Code, x.Name, x.Notes, x.ParentID, x.ApproveOnly, Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if e != nil {
		return fmt.Errorf("store: insert team: %w", e)
	}
	return nil
}
func TeamUpdate(ctx context.Context, q Querier, x model.Team) error {
	res, e := q.ExecContext(ctx, "UPDATE teams SET code=?,name=?,notes=?,parent_id=?,approve_only=?,updated_at=? WHERE id=? AND organization_chart_id=?", x.Code, x.Name, x.Notes, x.ParentID, x.ApproveOnly, Millis(x.UpdatedAt), x.ID, x.OrganizationChartID)
	return pf19RowsAffected(res, e)
}
func TeamDelete(ctx context.Context, q Querier, id, chartID string) error {
	res, e := q.ExecContext(ctx, "DELETE FROM teams WHERE id=? AND organization_chart_id=?", id, chartID)
	return pf19RowsAffected(res, e)
}
func TeamMembershipInsert(ctx context.Context, q Querier, x model.TeamMembership) error {
	grades, e := json.Marshal(x.GradeIDs)
	if e != nil {
		return e
	}
	_, e = q.ExecContext(ctx, "INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", x.ID, x.TeamID, x.UserID, x.Leader, string(grades), Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if e != nil {
		return fmt.Errorf("store: insert team membership: %w", e)
	}
	return nil
}
func TeamMembershipUpdate(ctx context.Context, q Querier, teamID, userID string, leader bool, grades []string, at time.Time) error {
	encoded, e := json.Marshal(grades)
	if e != nil {
		return e
	}
	res, e := q.ExecContext(ctx, "UPDATE team_memberships SET leader=?,grade_ids=?,updated_at=? WHERE team_id=? AND user_id=?", leader, string(encoded), Millis(at), teamID, userID)
	return pf19RowsAffected(res, e)
}
func TeamMembershipDelete(ctx context.Context, q Querier, teamID string, userIDs []string) error {
	for _, id := range userIDs {
		res, e := q.ExecContext(ctx, "DELETE FROM team_memberships WHERE team_id=? AND user_id=?", teamID, id)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			return ErrNotFound
		}
	}
	return nil
}
func TeamMembershipHasLeader(ctx context.Context, q Querier, teamID string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM team_memberships WHERE team_id=? AND leader=1", teamID).Scan(&n)
	return n > 0, e
}
func TeamHasChild(ctx context.Context, q Querier, id string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM teams WHERE parent_id=?", id).Scan(&n)
	return n > 0, e
}
func TeamMembershipExists(ctx context.Context, q Querier, teamID, userID string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM team_memberships WHERE team_id=? AND user_id=?", teamID, userID).Scan(&n)
	return n > 0, e
}
func TeamCodeExists(ctx context.Context, q Querier, chartID, code, except string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM teams WHERE organization_chart_id=? AND code=? AND id<>?", chartID, code, except).Scan(&n)
	return n > 0, e
}
func TeamParentInChart(ctx context.Context, q Querier, chartID, parentID string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM teams WHERE id=? AND organization_chart_id=?", parentID, chartID).Scan(&n)
	return n > 0, e
}
func TeamGradeIDsExist(ctx context.Context, q Querier, ids []string) (bool, error) {
	for _, id := range ids {
		var n int
		e := q.QueryRowContext(ctx, "SELECT count(*) FROM grades WHERE id=?", id).Scan(&n)
		if e != nil {
			return false, e
		}
		if n == 0 {
			return false, nil
		}
	}
	return len(ids) > 0, nil
}
func TeamActiveUsers(ctx context.Context, q Querier, ids []string) (bool, error) {
	for _, id := range ids {
		var status string
		e := q.QueryRowContext(ctx, "SELECT status FROM users WHERE id=?", id).Scan(&status)
		if e != nil {
			return false, e
		}
		if status != model.UserStatusActive {
			return false, nil
		}
	}
	return true, nil
}
func TeamParentWouldCycle(ctx context.Context, q Querier, teamID, parentID string) (bool, error) {
	for id := parentID; id != ""; {
		if id == teamID {
			return true, nil
		}
		x, e := GetTeam(ctx, q, id)
		if e != nil {
			return false, e
		}
		if x.ParentID == nil {
			break
		}
		id = *x.ParentID
	}
	return false, nil
}
func pf19RowsAffected(res sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("store: mutate org data: %w", err)
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
