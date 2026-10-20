package store

import (
	"context"
	"fmt"
	"punchflow/internal/model"
	"strings"
)

func GetCategory(ctx context.Context, q Querier, id string) (model.Category, error) {
	var x model.Category
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,name,created_at,updated_at FROM categories WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Name, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetFolder(ctx context.Context, q Querier, id string) (model.Folder, error) {
	var x model.Folder
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,name,description,parent_id,editable,created_at,updated_at FROM folders WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Name, &x.Description, &x.ParentID, &x.Editable, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func FolderFullName(ctx context.Context, q Querier, id string) (string, error) {
	x, e := GetFolder(ctx, q, id)
	if e != nil {
		return "", e
	}
	names := []string{x.Name}
	for x.ParentID != nil {
		x, e = GetFolder(ctx, q, *x.ParentID)
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
func GetFolderAncestors(ctx context.Context, q Querier, id string) ([]model.Folder, error) {
	x, e := GetFolder(ctx, q, id)
	if e != nil {
		return nil, e
	}
	out := []model.Folder{}
	seen := map[string]bool{id: true}
	for x.ParentID != nil {
		if seen[*x.ParentID] {
			return nil, fmt.Errorf("store: folder parent cycle at %s", *x.ParentID)
		}
		seen[*x.ParentID] = true
		x, e = GetFolder(ctx, q, *x.ParentID)
		if e != nil {
			return nil, e
		}
		out = append([]model.Folder{x}, out...)
	}
	return out, nil
}
func CountFolderWorkflows(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM workflows WHERE folder_id=? AND status!='deleted'`, id)
}
func CountFolderRoutes(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM routes WHERE folder_id=? AND status!='deleted'`, id)
}
func GetCategoryIDs(ctx context.Context, q Querier, workflowID string) ([]string, error) {
	d, err := WorkflowDefByID(ctx, q, workflowID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(d.CategoryCodes))
	for _, code := range d.CategoryCodes {
		id, e := CategoryIDByCode(ctx, q, code)
		if e == nil {
			ids = append(ids, id)
		} else if e != ErrNotFound {
			return nil, e
		}
	}
	return ids, nil
}

func GetFolderChildren(ctx context.Context, q Querier, id string) ([]model.Folder, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM folders WHERE parent_id=? ORDER BY code,id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query folder children: %w", err)
	}
	defer func() { _ = rows.Close() }()
	childIDs := []string{}
	for rows.Next() {
		var childID string
		if err = rows.Scan(&childID); err != nil {
			return nil, err
		}
		childIDs = append(childIDs, childID)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate folder children: %w", err)
	}
	if err = rows.Close(); err != nil {
		return nil, fmt.Errorf("store: close folder children rows: %w", err)
	}
	out := make([]model.Folder, 0, len(childIDs))
	for _, childID := range childIDs {
		x, e := GetFolder(ctx, q, childID)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, nil
}
func CategoryIDByCode(ctx context.Context, q Querier, code string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM categories WHERE code=?`, code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}
