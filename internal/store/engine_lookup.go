package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
)

func EngTenantMode(ctx context.Context, q Querier) (string, error) {
	var mode string
	err := q.QueryRowContext(ctx, `SELECT mode FROM tenant ORDER BY created_at LIMIT 1`).Scan(&mode)
	if err != nil {
		return "", fmt.Errorf("store: read engine tenant mode: %w", err)
	}
	return mode, nil
}

func EngActiveMemberships(ctx context.Context, q Querier, userID, teamID string) ([]model.TeamMembership, error) {
	query := `SELECT m.id,m.team_id,m.user_id,m.leader,m.grade_ids,m.created_at,m.updated_at FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE c.current=1 AND m.user_id=?`
	args := []any{userID}
	if teamID != "" {
		query += ` AND m.team_id=?`
		args = append(args, teamID)
	}
	query += ` ORDER BY t.code,m.created_at`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query engine memberships: %w", err)
	}
	defer rows.Close()
	out := []model.TeamMembership{}
	for rows.Next() {
		var x model.TeamMembership
		var leader int
		var grades string
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TeamID, &x.UserID, &leader, &grades, &c, &u); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(grades), &x.GradeIDs); err != nil {
			return nil, fmt.Errorf("store: decode engine membership grades: %w", err)
		}
		x.Leader = leader != 0
		x.CreatedAt = Time(c)
		x.UpdatedAt = Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}

func EngTeamAncestors(ctx context.Context, q Querier, teamID string) ([]model.Team, error) {
	out := []model.Team{}
	seen := map[string]bool{}
	for teamID != "" && !seen[teamID] {
		seen[teamID] = true
		x, err := GetTeam(ctx, q, teamID)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
		if x.ParentID == nil {
			break
		}
		teamID = *x.ParentID
	}
	return out, nil
}

func EngLeadersOfTeam(ctx context.Context, q Querier, teamID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.user_id FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE c.current=1 AND m.team_id=? AND m.leader=1 ORDER BY m.user_id`, teamID)
	if err != nil {
		return nil, fmt.Errorf("store: query engine team leaders: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func EngUsersByTeam(ctx context.Context, q Querier, teamID string, descendants bool) ([]string, error) {
	ids := []string{teamID}
	if descendants {
		for i := 0; i < len(ids); i++ {
			children, err := GetTeamChildren(ctx, q, ids[i])
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				ids = append(ids, child.ID)
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		members, err := GetTeamMemberships(ctx, q, id)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if !seen[m.UserID] {
				seen[m.UserID] = true
				out = append(out, m.UserID)
			}
		}
	}
	return out, nil
}

func EngActiveProxyApprovers(ctx context.Context, q Querier, targetUserID, proxyUserID, workflowID, date string) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT starts_on,ends_on,workflow_ids FROM proxy_approvers WHERE user_id=? AND proxy_user_id=?`, targetUserID, proxyUserID)
	if err != nil {
		return false, fmt.Errorf("store: query engine proxy approvers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var starts, ends *string
		var raw string
		if err = rows.Scan(&starts, &ends, &raw); err != nil {
			return false, err
		}
		if starts != nil && *starts > date || ends != nil && *ends < date {
			continue
		}
		var workflows []string
		if err = json.Unmarshal([]byte(raw), &workflows); err != nil {
			return false, fmt.Errorf("store: decode proxy workflow ids: %w", err)
		}
		if len(workflows) == 0 || pf14Has(workflows, workflowID) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func EngSignedFileExists(ctx context.Context, q Querier, signedID string) (bool, error) {
	var found int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM files WHERE signed_id=?`, signedID).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: lookup signed file: %w", err)
	}
	return true, nil
}

func EngUserIsActiveMember(ctx context.Context, q Querier, userID, teamID string) (bool, error) {
	members, err := EngActiveMemberships(ctx, q, userID, teamID)
	return len(members) > 0, err
}

func pf14Has(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
