package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"punchflow/internal/model"
)

// InvalidateRoutesReferencing marks visible routes invalid when a deleted
// user or team is a directly configured approver/approval target. Workflows
// selecting each affected route are made unavailable in the same transaction.
func InvalidateRoutesReferencing(ctx context.Context, q Querier, code string, user bool, at time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT id,code,definition_json FROM routes WHERE status!=? ORDER BY id`, model.RouteStatusDeleted)
	if err != nil {
		return fmt.Errorf("store: list routes for deletion reconciliation: %w", err)
	}
	type routeRow struct {
		id, code string
		body     []byte
	}
	var routes []routeRow
	for rows.Next() {
		var route routeRow
		if err := rows.Scan(&route.id, &route.code, &route.body); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: scan route for deletion reconciliation: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store: iterate routes for deletion reconciliation: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("store: close routes for deletion reconciliation: %w", err)
	}
	for _, row := range routes {
		var def model.RouteDef
		if err := json.Unmarshal(row.body, &def); err != nil {
			return fmt.Errorf("store: decode route %s for deletion reconciliation: %w", row.code, err)
		}
		if !pfReconcileRouteReferences(def, code, user) {
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE routes SET status=?,updated_at=? WHERE id=?`, model.RouteStatusError, Millis(at), row.id); err != nil {
			return fmt.Errorf("store: invalidate route %s: %w", row.code, err)
		}
		if _, err := q.ExecContext(ctx, `UPDATE workflows SET status=?,updated_at=? WHERE status=? AND json_extract(definition_json,'$.routeCode')=?`, model.WorkflowStatusInvisible, Millis(at), model.WorkflowStatusVisible, row.code); err != nil {
			return fmt.Errorf("store: disable workflows for route %s: %w", row.code, err)
		}
	}
	return nil
}

func pfReconcileRouteReferences(def model.RouteDef, code string, user bool) bool {
	for _, step := range def.Steps {
		if user {
			for _, userCode := range step.UserCodes {
				if userCode == code {
					return true
				}
			}
			continue
		}
		for _, target := range step.Targets {
			if target.TeamCode == code {
				return true
			}
		}
	}
	return false
}

// RemoveTeamFromRoleTargets removes a deleted team from every administrator
// role permission target while preserving unrelated permission properties.
func RemoveTeamFromRoleTargets(ctx context.Context, q Querier, teamID string, at time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT id,permission_list FROM roles ORDER BY id`)
	if err != nil {
		return fmt.Errorf("store: list roles for team cleanup: %w", err)
	}
	type roleRow struct{ id, raw string }
	var roles []roleRow
	for rows.Next() {
		var role roleRow
		if err := rows.Scan(&role.id, &role.raw); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: scan role for team cleanup: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store: iterate roles for team cleanup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("store: close roles for team cleanup: %w", err)
	}
	for _, role := range roles {
		var permissions []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(role.raw), &permissions); err != nil {
			return fmt.Errorf("store: decode role %s permissions: %w", role.id, err)
		}
		changed := false
		for _, permission := range permissions {
			raw, exists := permission["teamIds"]
			if !exists {
				continue
			}
			var ids []string
			if err := json.Unmarshal(raw, &ids); err != nil {
				return fmt.Errorf("store: decode role %s team targets: %w", role.id, err)
			}
			kept := make([]string, 0, len(ids))
			for _, id := range ids {
				if id == teamID {
					changed = true
					continue
				}
				kept = append(kept, id)
			}
			if changed {
				encoded, err := json.Marshal(kept)
				if err != nil {
					return err
				}
				permission["teamIds"] = encoded
			}
		}
		if changed {
			encoded, err := json.Marshal(permissions)
			if err != nil {
				return fmt.Errorf("store: encode role %s permissions: %w", role.id, err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE roles SET permission_list=?,updated_at=? WHERE id=?`, string(encoded), Millis(at), role.id); err != nil {
				return fmt.Errorf("store: update role %s team targets: %w", role.id, err)
			}
		}
	}
	return nil
}
