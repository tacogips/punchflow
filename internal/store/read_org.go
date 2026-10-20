package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
	"strings"
)

func GetOrganizationChart(ctx context.Context, q Querier, id string) (model.OrganizationChart, error) {
	var x model.OrganizationChart
	var current, cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,name,current,created_at,updated_at FROM organization_charts WHERE id=?`, id).Scan(&x.ID, &x.Name, &current, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Current = current != 0
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func ActiveOrgChartID(ctx context.Context, q Querier) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM organization_charts WHERE current=1 ORDER BY created_at DESC LIMIT 1`).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}
func GetTeam(ctx context.Context, q Querier, id string) (model.Team, error) {
	var x model.Team
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,organization_chart_id,code,name,notes,parent_id,approve_only,created_at,updated_at FROM teams WHERE id=?`, id).Scan(&x.ID, &x.OrganizationChartID, &x.Code, &x.Name, &x.Notes, &x.ParentID, &x.ApproveOnly, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetTeams(ctx context.Context, q Querier, ids []string) ([]model.Team, error) {
	out := make([]model.Team, 0, len(ids))
	for _, id := range ids {
		x, e := GetTeam(ctx, q, id)
		if e == nil {
			out = append(out, x)
		} else if e != ErrNotFound {
			return nil, e
		}
	}
	return out, nil
}
func TeamIDByCode(ctx context.Context, q Querier, chartID, code string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM teams WHERE organization_chart_id=? AND code=?`, chartID, code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}
func TeamFullName(ctx context.Context, q Querier, id string) (string, error) {
	x, e := GetTeam(ctx, q, id)
	if e != nil {
		return "", e
	}
	names := []string{x.Name}
	for x.ParentID != nil {
		x, e = GetTeam(ctx, q, *x.ParentID)
		if e != nil {
			return "", e
		}
		names = append(names, x.Name)
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return strings.Join(names, "/"), nil
}
func GetTeamChildren(ctx context.Context, q Querier, id string) ([]model.Team, error) {
	rows, e := q.QueryContext(ctx, `SELECT id FROM teams WHERE parent_id=? ORDER BY code,id`, id)
	if e != nil {
		return nil, fmt.Errorf("store: query team children: %w", e)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		ids = append(ids, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return GetTeams(ctx, q, ids)
}
func GetTeamMemberships(ctx context.Context, q Querier, teamID string) ([]model.TeamMembership, error) {
	rows, e := q.QueryContext(ctx, `SELECT id,team_id,user_id,leader,grade_ids,created_at,updated_at FROM team_memberships WHERE team_id=? ORDER BY created_at,user_id`, teamID)
	if e != nil {
		return nil, fmt.Errorf("store: query memberships: %w", e)
	}
	defer rows.Close()
	out := []model.TeamMembership{}
	for rows.Next() {
		var x model.TeamMembership
		var leader, cr, up int64
		var grades string
		if e = rows.Scan(&x.ID, &x.TeamID, &x.UserID, &leader, &grades, &cr, &up); e != nil {
			return nil, e
		}
		x.Leader = leader != 0
		x.CreatedAt = Time(cr)
		x.UpdatedAt = Time(up)
		if e = json.Unmarshal([]byte(grades), &x.GradeIDs); e != nil {
			return nil, fmt.Errorf("store: decode membership grades: %w", e)
		}
		if x.GradeIDs == nil {
			x.GradeIDs = []string{}
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func CountTeamUsers(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM team_memberships WHERE team_id=?`, id)
}
func CountOrgChartTeams(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM teams WHERE organization_chart_id=?`, id)
}
func CountOrgChartMemberships(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM team_memberships m JOIN teams t ON t.id=m.team_id WHERE t.organization_chart_id=?`, id)
}
func pf08Count(ctx context.Context, q Querier, query string, args ...any) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, query, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count rows: %w", err)
	}
	return n, nil
}

var _ = sql.ErrNoRows
