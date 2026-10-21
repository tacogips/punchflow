package store

import (
	"context"
	"database/sql"
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
		if !user && removeNominationTeamTarget(&def, code) {
			encoded, err := json.Marshal(def)
			if err != nil {
				return fmt.Errorf("store: encode reconciled route %s: %w", row.code, err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE routes SET definition_json=?,updated_at=? WHERE id=?`, string(encoded), Millis(at), row.id); err != nil {
				return fmt.Errorf("store: remove nomination target from route %s: %w", row.code, err)
			}
		}
		if !pfReconcileRouteReferences(def, code, user) {
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE routes SET status=?,updated_at=? WHERE id=?`, model.RouteStatusError, Millis(at), row.id); err != nil {
			return fmt.Errorf("store: invalidate route %s: %w", row.code, err)
		}
		if err := invalidateWorkflowsForRoute(ctx, q, row.code, at); err != nil {
			return fmt.Errorf("store: disable workflows for route %s: %w", row.code, err)
		}
	}
	return nil
}

func removeNominationTeamTarget(def *model.RouteDef, code string) bool {
	changed := false
	for i := range def.Steps {
		step := &def.Steps[i]
		if step.StepType != "author_customizable" && step.StepType != "assignee_customizable" {
			continue
		}
		kept := step.Targets[:0]
		for _, target := range step.Targets {
			if target.TeamCode == code {
				changed = true
				continue
			}
			kept = append(kept, target)
		}
		step.Targets = kept
	}
	return changed
}

// RemoveTeamDefinitionReferences removes settings whose team target no longer
// exists, including workflow eligibility, viewers, watchers and form targets.
func RemoveTeamDefinitionReferences(ctx context.Context, q Querier, code string, at time.Time) error {
	return rewriteDefinitions(ctx, q, at, func(table string, raw []byte) ([]byte, bool, error) {
		if table == "routes" {
			var def model.RouteDef
			if err := json.Unmarshal(raw, &def); err != nil {
				return nil, false, err
			}
			changed := removeNominationTeamTarget(&def, code)
			if !changed {
				return raw, false, nil
			}
			encoded, err := json.Marshal(def)
			return encoded, true, err
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, err
		}
		changed := removeJSONReference(&value, "teamCode", code)
		if !changed {
			return raw, false, nil
		}
		encoded, err := json.Marshal(value)
		return encoded, true, err
	})
}

func removeJSONReference(value *any, key, target string) bool {
	changed := false
	switch node := (*value).(type) {
	case map[string]any:
		for k, v := range node {
			if k == key && v == target {
				delete(node, k)
				changed = true
				continue
			}
			child := v
			if removeJSONReference(&child, key, target) {
				node[k] = child
				changed = true
			}
		}
	case []any:
		kept := node[:0]
		for _, item := range node {
			if m, ok := item.(map[string]any); ok && m[key] == target {
				changed = true
				continue
			}
			child := item
			if removeJSONReference(&child, key, target) {
				item = child
				changed = true
			}
			kept = append(kept, item)
		}
		*value = kept
	}
	return changed
}

// RemoveGradeDefinitionReferences drops a deleted grade from route and
// workflow filters so those stored definitions remain readable.
func RemoveGradeDefinitionReferences(ctx context.Context, q Querier, code string, at time.Time) error {
	return rewriteDefinitions(ctx, q, at, func(_ string, raw []byte) ([]byte, bool, error) {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, err
		}
		changed := reconcileGradeJSON(&value, code)
		if !changed {
			return raw, false, nil
		}
		encoded, err := json.Marshal(value)
		return encoded, true, err
	})
}

func reconcileGradeJSON(value *any, code string) bool {
	changed := false
	switch node := (*value).(type) {
	case map[string]any:
		for key, childValue := range node {
			if key == "gradeCode" && childValue == code {
				delete(node, key)
				changed = true
				continue
			}
			if key == "gradeCodes" {
				if list, ok := childValue.([]any); ok {
					kept := list[:0]
					removed := false
					for _, item := range list {
						if item == code {
							removed = true
							changed = true
							continue
						}
						kept = append(kept, item)
					}
					if removed && len(kept) == 0 {
						delete(node, key)
						delete(node, "gradeSymbol")
					} else {
						node[key] = kept
					}
				}
				continue
			}
			child := childValue
			if reconcileGradeJSON(&child, code) {
				node[key] = child
				changed = true
			}
		}
	case []any:
		kept := node[:0]
		for _, item := range node {
			if object, ok := item.(map[string]any); ok {
				if object["gradeCode"] == code {
					changed = true
					continue
				}
				if grades, ok := object["gradeCodes"].([]any); ok && len(grades) == 1 && grades[0] == code && object["teamCode"] == nil {
					changed = true
					continue
				}
			}
			child := item
			if reconcileGradeJSON(&child, code) {
				item = child
				changed = true
			}
			kept = append(kept, item)
		}
		*value = kept
	}
	return changed
}

// ReconcileDefinitionCodeChange cascades a resource code rename through stored
// fixture definitions. IDs remain stable; only fields that represent the
// renamed resource's code are changed.
func ReconcileDefinitionCodeChange(ctx context.Context, q Querier, kind, oldCode, newCode string, at time.Time) error {
	if oldCode == newCode {
		return nil
	}
	keySets := map[string]map[string]bool{
		"team":        {"teamCode": true, "teamCodes": true},
		"user":        {"userCode": true, "userCodes": true},
		"grade":       {"gradeCode": true, "gradeCodes": true},
		"master":      {"generalMasterCode": true},
		"master_item": {"generalMasterItemCode": true},
	}
	keys := keySets[kind]
	if len(keys) == 0 {
		return fmt.Errorf("unsupported definition code kind %q", kind)
	}
	return rewriteDefinitions(ctx, q, at, func(_ string, raw []byte) ([]byte, bool, error) {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, err
		}
		changed := replaceJSONCode(&value, keys, oldCode, newCode)
		if !changed {
			return raw, false, nil
		}
		encoded, err := json.Marshal(value)
		return encoded, true, err
	})
}
func replaceJSONCode(value *any, keys map[string]bool, oldCode, newCode string) bool {
	changed := false
	switch node := (*value).(type) {
	case map[string]any:
		for k, v := range node {
			if keys[k] {
				if scalar, ok := v.(string); ok && scalar == oldCode {
					node[k] = newCode
					changed = true
					continue
				}
				if list, ok := v.([]any); ok {
					for i, item := range list {
						if item == oldCode {
							list[i] = newCode
							changed = true
						}
					}
					node[k] = list
					continue
				}
			}
			child := v
			if replaceJSONCode(&child, keys, oldCode, newCode) {
				node[k] = child
				changed = true
			}
		}
	case []any:
		for i := range node {
			child := node[i]
			if replaceJSONCode(&child, keys, oldCode, newCode) {
				node[i] = child
				changed = true
			}
		}
	}
	return changed
}

func rewriteDefinitions(ctx context.Context, q Querier, at time.Time, rewrite func(string, []byte) ([]byte, bool, error)) error {
	for _, table := range []string{"routes", "workflows"} {
		rows, err := q.QueryContext(ctx, "SELECT id,definition_json FROM "+table+" ORDER BY id")
		if err != nil {
			return err
		}
		type row struct {
			id  string
			raw []byte
		}
		var all []row
		for rows.Next() {
			var x row
			if err := rows.Scan(&x.id, &x.raw); err != nil {
				_ = rows.Close()
				return err
			}
			all = append(all, x)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, x := range all {
			encoded, changed, err := rewrite(table, x.raw)
			if err != nil {
				return fmt.Errorf("decode %s %s for reconciliation: %w", table, x.id, err)
			}
			if !changed {
				continue
			}
			if _, err := q.ExecContext(ctx, "UPDATE "+table+" SET definition_json=?,updated_at=? WHERE id=?", string(encoded), Millis(at), x.id); err != nil {
				return err
			}
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
		// Nomination restrictions are advisory candidate filters. Kickflow
		// removes them when a team is deleted; only fixed team approvers make
		// the route unusable.
		if step.StepType == "author_customizable" || step.StepType == "assignee_customizable" {
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

func invalidateWorkflowsForRoute(ctx context.Context, q Querier, routeCode string, at time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT id,definition_json FROM workflows WHERE status=? ORDER BY id`, model.WorkflowStatusVisible)
	if err != nil {
		return fmt.Errorf("list workflows for route reconciliation: %w", err)
	}
	type workflowRow struct {
		id   string
		body []byte
	}
	var workflows []workflowRow
	for rows.Next() {
		var row workflowRow
		if err := rows.Scan(&row.id, &row.body); err != nil {
			_ = rows.Close()
			return err
		}
		workflows = append(workflows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range workflows {
		var definition model.WorkflowDef
		if err := json.Unmarshal(row.body, &definition); err != nil {
			return fmt.Errorf("decode workflow %s: %w", row.id, err)
		}
		depends := definition.RouteCode == routeCode
		for _, condition := range definition.RouteConditions {
			depends = depends || (condition.RouteCode != nil && *condition.RouteCode == routeCode)
		}
		if !depends {
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE workflows SET status=?,updated_at=? WHERE id=?`, model.WorkflowStatusInvisible, Millis(at), row.id); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTeamFromRoleTargets removes a deleted team from every administrator
// role permission target while preserving unrelated permission properties.
func RemoveTeamFromRoleTargets(ctx context.Context, q Querier, teamID string, at time.Time) error {
	return removeRoleTarget(ctx, q, "teamIds", teamID, at)
}

// RemoveGeneralMasterFromRoleTargets removes a deleted master from role scopes.
func RemoveGeneralMasterFromRoleTargets(ctx context.Context, q Querier, masterID string, at time.Time) error {
	return removeRoleTarget(ctx, q, "generalMasterIds", masterID, at)
}

// RemoveFoldersFromRoleTargets removes deleted folders from role scopes.
func RemoveFoldersFromRoleTargets(ctx context.Context, q Querier, folderIDs []string, at time.Time) error {
	for _, id := range folderIDs {
		if err := removeRoleTarget(ctx, q, "folderIds", id, at); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileRoleTeamTargets replaces old-chart team IDs with matching new-chart
// IDs and drops targets whose team code is absent from the new chart.
func ReconcileRoleTeamTargets(ctx context.Context, q Querier, oldChartID, newChartID string, at time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT old_team.id,old_team.code,new_team.id FROM teams old_team LEFT JOIN teams new_team ON new_team.code=old_team.code AND new_team.organization_chart_id=? WHERE old_team.organization_chart_id=?`, newChartID, oldChartID)
	if err != nil {
		return fmt.Errorf("list chart team mapping for role reconciliation: %w", err)
	}
	mapping := map[string]string{}
	for rows.Next() {
		var oldID, code string
		var newID sql.NullString
		if err := rows.Scan(&oldID, &code, &newID); err != nil {
			_ = rows.Close()
			return err
		}
		if newID.Valid {
			mapping[oldID] = newID.String
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	roles, err := q.QueryContext(ctx, `SELECT id,permission_list FROM roles ORDER BY id`)
	if err != nil {
		return err
	}
	type row struct{ id, raw string }
	var all []row
	for roles.Next() {
		var r row
		if err := roles.Scan(&r.id, &r.raw); err != nil {
			_ = roles.Close()
			return err
		}
		all = append(all, r)
	}
	if err := roles.Err(); err != nil {
		_ = roles.Close()
		return err
	}
	if err := roles.Close(); err != nil {
		return err
	}
	for _, role := range all {
		var permissions []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(role.raw), &permissions); err != nil {
			return fmt.Errorf("decode role %s permissions: %w", role.id, err)
		}
		changed := false
		for _, permission := range permissions {
			raw, ok := permission["teamIds"]
			if !ok {
				continue
			}
			var ids []string
			if err := json.Unmarshal(raw, &ids); err != nil {
				return err
			}
			next := make([]string, 0, len(ids))
			for _, id := range ids {
				mapped, exists := mapping[id]
				if !exists {
					changed = true
					continue
				}
				if mapped != id {
					changed = true
				}
				next = append(next, mapped)
			}
			if changed {
				encoded, err := json.Marshal(next)
				if err != nil {
					return err
				}
				permission["teamIds"] = encoded
			}
		}
		if changed {
			encoded, err := json.Marshal(permissions)
			if err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE roles SET permission_list=?,updated_at=? WHERE id=?`, string(encoded), Millis(at), role.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeRoleTarget(ctx context.Context, q Querier, targetKey, targetID string, at time.Time) error {
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
			raw, exists := permission[targetKey]
			if !exists || targetKey != "teamIds" && targetKey != "generalMasterIds" && targetKey != "folderIds" {
				continue
			}
			var ids []string
			if err := json.Unmarshal(raw, &ids); err != nil {
				return fmt.Errorf("store: decode role %s %s targets: %w", role.id, targetKey, err)
			}
			kept := make([]string, 0, len(ids))
			for _, id := range ids {
				if id == targetID {
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
				permission[targetKey] = encoded
			}
		}
		if changed {
			encoded, err := json.Marshal(permissions)
			if err != nil {
				return fmt.Errorf("store: encode role %s permissions: %w", role.id, err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE roles SET permission_list=?,updated_at=? WHERE id=?`, string(encoded), Millis(at), role.id); err != nil {
				return fmt.Errorf("store: update role %s %s targets: %w", role.id, targetKey, err)
			}
		}
	}
	return nil
}
