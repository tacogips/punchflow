package present

import (
	"context"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func Grade(m model.Grade) GradeJSON {
	return GradeJSON{"id": m.ID, "code": m.Code, "name": m.Name, "level": m.Level, "isDefault": m.IsDefault, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}
}
func Grades(ctx context.Context, q store.Querier, ids []string) ([]GradeJSON, error) {
	xs, e := store.GetGrades(ctx, q, ids)
	if e != nil {
		return nil, e
	}
	out := make([]GradeJSON, 0, len(xs))
	for _, x := range xs {
		out = append(out, Grade(x))
	}
	return out, nil
}
func Team(ctx context.Context, q store.Querier, id string) (TeamJSON, error) {
	m, e := store.GetTeam(ctx, q, id)
	if e != nil {
		return nil, e
	}
	name, e := store.TeamFullName(ctx, q, id)
	if e != nil {
		return nil, e
	}
	n, e := store.CountTeamUsers(ctx, q, id)
	if e != nil {
		return nil, e
	}
	return TeamJSON{"id": m.ID, "name": m.Name, "fullName": name, "code": m.Code, "notes": m.Notes, "approveOnly": m.ApproveOnly, "usersCount": n, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}, nil
}
func TeamDetail(ctx context.Context, q store.Querier, id string) (TeamDetailJSON, error) {
	base, e := Team(ctx, q, id)
	if e != nil {
		return nil, e
	}
	m, e := store.GetTeam(ctx, q, id)
	if e != nil {
		return nil, e
	}
	children, e := store.GetTeamChildren(ctx, q, id)
	if e != nil {
		return nil, e
	}
	childJSON := []any{}
	for _, c := range children {
		x, er := Team(ctx, q, c.ID)
		if er != nil {
			return nil, er
		}
		childJSON = append(childJSON, x)
	}
	var parent any
	if m.ParentID != nil {
		parent, e = Team(ctx, q, *m.ParentID)
		if e != nil {
			return nil, e
		}
	}
	members, e := store.GetTeamMemberships(ctx, q, id)
	if e != nil {
		return nil, e
	}
	users := []any{}
	for i, member := range members {
		if i == 100 {
			break
		}
		u, er := User(ctx, q, member.UserID, UserOpts{})
		if er != nil {
			return nil, er
		}
		grades, er := Grades(ctx, q, member.GradeIDs)
		if er != nil {
			return nil, er
		}
		mu := MemberUserJSON(u)
		mu["grades"] = pf08Array(grades)
		mu["leader"] = member.Leader
		users = append(users, mu)
	}
	out := TeamDetailJSON(base)
	out["parent"] = parent
	out["children"] = childJSON
	out["users"] = users
	return out, nil
}
func MemberUsers(ctx context.Context, q store.Querier, teamID string, offset, limit int) ([]MemberUserJSON, error) {
	ms, e := store.GetTeamMemberships(ctx, q, teamID)
	if e != nil {
		return nil, e
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	out := []MemberUserJSON{}
	for i, m := range ms {
		if i < offset {
			continue
		}
		if len(out) >= limit {
			break
		}
		u, e := User(ctx, q, m.UserID, UserOpts{})
		if e != nil {
			return nil, e
		}
		g, e := Grades(ctx, q, m.GradeIDs)
		if e != nil {
			return nil, e
		}
		mu := MemberUserJSON(u)
		mu["grades"] = pf08Array(g)
		mu["leader"] = m.Leader
		out = append(out, mu)
	}
	return out, nil
}
func OrganizationChart(ctx context.Context, q store.Querier, id string) (OrganizationChartJSON, error) {
	x, e := store.GetOrganizationChart(ctx, q, id)
	if e != nil {
		return nil, e
	}
	tc, e := store.CountOrgChartTeams(ctx, q, id)
	if e != nil {
		return nil, e
	}
	mc, e := store.CountOrgChartMemberships(ctx, q, id)
	if e != nil {
		return nil, e
	}
	return OrganizationChartJSON{"id": x.ID, "name": x.Name, "current": x.Current, "teamsCount": tc, "membershipsCount": mc, "createdAt": pf08Time(x.CreatedAt), "updatedAt": pf08Time(x.UpdatedAt), "activationPlan": nil}, nil
}
func OrganizationChartDetail(ctx context.Context, q store.Querier, id string) (OrganizationChartJSON, error) {
	return OrganizationChart(ctx, q, id)
}
