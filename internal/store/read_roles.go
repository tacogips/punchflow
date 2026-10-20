package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
)

func GetRole(ctx context.Context, q Querier, id string) (model.Role, error) {
	var x model.Role
	var editable, cr, up int64
	var perms string
	err := q.QueryRowContext(ctx, `SELECT id,name,editable,permission_list,created_at,updated_at FROM roles WHERE id=?`, id).Scan(&x.ID, &x.Name, &editable, &perms, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Editable = editable != 0
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	if err = json.Unmarshal([]byte(perms), &x.PermissionList); err != nil {
		return x, fmt.Errorf("store: decode role permissions: %w", err)
	}
	if x.PermissionList == nil {
		x.PermissionList = []model.RolePermission{}
	}
	return x, nil
}
func CountRoleUsers(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM role_memberships WHERE role_id=?`, id)
}
func GetRoleUserIDs(ctx context.Context, q Querier, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id FROM role_memberships WHERE role_id=? ORDER BY user_id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query role memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ = sql.ErrNoRows
