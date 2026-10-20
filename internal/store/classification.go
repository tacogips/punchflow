package store

import (
	"context"
	"fmt"
	"punchflow/internal/model"
)

func CategoryList(ctx context.Context, q Querier, offset, limit int, sortBy string, desc bool) ([]model.Category, int, error) {
	order := map[string]string{"name": "name", "createdAt": "created_at", "updatedAt": "updated_at"}[sortBy]
	if order == "" {
		order = "name"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM categories`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count categories: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM categories ORDER BY `+order+`,id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list categories: %w", err)
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
	out := make([]model.Category, 0, len(ids))
	for _, id := range ids {
		x, e := GetCategory(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	return out, total, nil
}
func CreateCategory(ctx context.Context, q Querier, x model.Category) error {
	_, err := q.ExecContext(ctx, `INSERT INTO categories(id,code,name,created_at,updated_at) VALUES(?,?,?,?,?)`, x.ID, x.Code, x.Name, Millis(x.CreatedAt), Millis(x.UpdatedAt))
	return err
}
func UpdateCategory(ctx context.Context, q Querier, id, name string, at int64) error {
	res, err := q.ExecContext(ctx, `UPDATE categories SET name=?,updated_at=? WHERE id=?`, name, at, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func DeleteCategory(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM categories WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func FolderList(ctx context.Context, q Querier, offset, limit int, sortBy string, desc bool) ([]model.Folder, int, error) {
	order := map[string]string{"createdAt": "created_at", "name": "name", "fullName": "name"}[sortBy]
	if order == "" {
		order = "name"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM folders`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM folders ORDER BY `+order+`,id LIMIT ? OFFSET ?`, limit, offset)
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
	out := make([]model.Folder, 0, len(ids))
	for _, id := range ids {
		x, e := GetFolder(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	return out, total, nil
}
func CreateFolder(ctx context.Context, q Querier, x model.Folder) error {
	_, err := q.ExecContext(ctx, `INSERT INTO folders(id,code,name,description,parent_id,editable,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, x.Code, x.Name, x.Description, x.ParentID, x.Editable, Millis(x.CreatedAt), Millis(x.UpdatedAt))
	return err
}
func UpdateFolder(ctx context.Context, q Querier, x model.Folder) error {
	res, err := q.ExecContext(ctx, `UPDATE folders SET code=?,name=?,description=?,parent_id=?,updated_at=? WHERE id=?`, x.Code, x.Name, x.Description, x.ParentID, Millis(x.UpdatedAt), x.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func DeleteFolder(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM folders WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func FolderHasChildren(ctx context.Context, q Querier, id string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM folders WHERE parent_id=?`, id).Scan(&n)
	return n > 0, err
}
func FolderHasDefinitions(ctx context.Context, q Querier, id string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM workflows WHERE folder_id=? AND status!='deleted')+(SELECT count(*) FROM routes WHERE folder_id=? AND status!='deleted')`, id, id).Scan(&n)
	return n > 0, err
}
