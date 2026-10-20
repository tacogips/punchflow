package store

import (
	"context"
	"fmt"
	"punchflow/internal/model"
)

func RouteList(ctx context.Context, q Querier, offset, limit int, status, folderID, sortBy string, desc bool) ([]model.Route, int, error) {
	return pf20ListDefinitionIDs(ctx, q, "routes", offset, limit, status, folderID, sortBy, desc)
}
func WorkflowList(ctx context.Context, q Querier, offset, limit int, status, folderID string, categoryCode, sortBy string, desc bool) ([]model.Workflow, int, error) {
	where := ` WHERE status!='deleted'`
	args := []any{}
	if status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	if folderID != "" {
		where += ` AND folder_id=?`
		args = append(args, folderID)
	}
	// Category membership lives in the immutable fixture JSON, so filter it after reading candidates.
	rows, err := q.QueryContext(ctx, `SELECT id FROM workflows`+where+` ORDER BY `+pf20DefinitionOrder(sortBy, desc)+`,id`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list workflows: %w", err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	all := []model.Workflow{}
	for _, id := range ids {
		x, e := GetWorkflow(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		if categoryCode != "" {
			d, e := WorkflowDefByID(ctx, q, id)
			if e != nil {
				return nil, 0, e
			}
			found := false
			for _, code := range d.CategoryCodes {
				if code == categoryCode {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		all = append(all, x)
	}
	total := len(all)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}
func pf20ListDefinitionIDs(ctx context.Context, q Querier, table string, offset, limit int, status, folderID, sortBy string, desc bool) ([]model.Route, int, error) {
	if table != "routes" && table != "workflows" {
		return nil, 0, fmt.Errorf("store: invalid definition table")
	}
	where := ` WHERE status!='deleted'`
	args := []any{}
	if status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	if folderID != "" {
		where += ` AND folder_id=?`
		args = append(args, folderID)
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM `+table+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM `+table+where+` ORDER BY `+pf20DefinitionOrder(sortBy, desc)+`,id LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	out := make([]model.Route, 0, len(ids))
	for _, id := range ids {
		x, e := GetRoute(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	return out, total, nil
}
func pf20DefinitionOrder(sortBy string, desc bool) string {
	order := map[string]string{"name": "json_extract(definition_json, '$.name')", "code": "code", "createdAt": "created_at", "updatedAt": "updated_at", "status": "status", "folderId": "folder_id"}[sortBy]
	if order == "" {
		order = "code"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	return order
}
