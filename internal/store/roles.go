package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"punchflow/internal/model"
)

// RoleList returns role IDs and the total count in deterministic order.
func RoleList(ctx context.Context, q Querier, offset, limit int, sortField string, desc bool) ([]string, int, error) {
	order := "name ASC, id ASC"
	if sortField == "createdAt" {
		order = "created_at"
		if desc {
			order += " DESC"
		} else {
			order += " ASC"
		}
	} else if sortField == "name" {
		order = "name"
		if desc {
			order += " DESC"
		} else {
			order += " ASC"
		}
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM roles`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count roles: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM roles ORDER BY `+order+` LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list roles: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan role id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate roles: %w", err)
	}
	return ids, total, nil
}

// RolePermissionJSON returns the stored permission list, including API-only scopes.
func RolePermissionJSON(ctx context.Context, q Querier, id string) (json.RawMessage, error) {
	var raw string
	if err := q.QueryRowContext(ctx, `SELECT permission_list FROM roles WHERE id=?`, id).Scan(&raw); err != nil {
		return nil, pf08NotFound(err)
	}
	return json.RawMessage(raw), nil
}

// RoleCreate inserts a role with the caller-provided API permission representation.
func RoleCreate(ctx context.Context, q Querier, role model.Role, permissions json.RawMessage) error {
	if len(permissions) == 0 {
		permissions = json.RawMessage("[]")
	}
	editable := int64(0)
	if role.Editable {
		editable = 1
	}
	_, err := q.ExecContext(ctx, `INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES(?,?,?,?,?,?)`, role.ID, role.Name, editable, string(permissions), Millis(role.CreatedAt), Millis(role.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create role: %w", err)
	}
	return nil
}

// RoleUpdate updates a role's mutable fields.
func RoleUpdate(ctx context.Context, q Querier, id string, name *string, permissions json.RawMessage, updatedAt int64) error {
	sets := []string{}
	args := []any{}
	if name != nil {
		sets = append(sets, "name=?")
		args = append(args, *name)
	}
	if permissions != nil {
		sets = append(sets, "permission_list=?")
		args = append(args, string(permissions))
	}
	sets = append(sets, "updated_at=?")
	args = append(args, updatedAt, id)
	result, err := q.ExecContext(ctx, `UPDATE roles SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	if err != nil {
		return fmt.Errorf("store: update role: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count updated roles: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RoleDelete removes an editable role.
func RoleDelete(ctx context.Context, q Querier, id string) error {
	result, err := q.ExecContext(ctx, `DELETE FROM roles WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("store: delete role: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count deleted roles: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RoleMemberIDs returns user IDs in stable order.
func RoleMemberIDs(ctx context.Context, q Querier, roleID string, offset, limit int) ([]string, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM role_memberships WHERE role_id=?`, roleID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count role members: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT user_id FROM role_memberships WHERE role_id=? ORDER BY user_id LIMIT ? OFFSET ?`, roleID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list role members: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan role member: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate role members: %w", err)
	}
	return ids, total, nil
}

// RoleHasMember reports whether the user belongs to the role.
func RoleHasMember(ctx context.Context, q Querier, roleID, userID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM role_memberships WHERE role_id=? AND user_id=?`, roleID, userID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check role membership: %w", err)
	}
	return n != 0, nil
}

// RoleIsSuperAdminMember reports whether the user belongs to any non-editable role.
func RoleIsSuperAdminMember(ctx context.Context, q Querier, userID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM role_memberships m JOIN roles r ON r.id=m.role_id WHERE m.user_id=? AND r.editable=0`, userID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check super admin membership: %w", err)
	}
	return n != 0, nil
}

// RoleAddMembers inserts the memberships; existing membership pairs are ignored.
func RoleAddMembers(ctx context.Context, q Querier, roleID string, userIDs []string, createdAt int64) error {
	for _, userID := range userIDs {
		if _, err := q.ExecContext(ctx, `INSERT INTO role_memberships(role_id,user_id,created_at) VALUES(?,?,?) ON CONFLICT(role_id,user_id) DO NOTHING`, roleID, userID, createdAt); err != nil {
			return fmt.Errorf("store: add role member: %w", err)
		}
	}
	return nil
}

// RoleRemoveMember deletes one role membership.
func RoleRemoveMember(ctx context.Context, q Querier, roleID, userID string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM role_memberships WHERE role_id=? AND user_id=?`, roleID, userID); err != nil {
		return fmt.Errorf("store: remove role member: %w", err)
	}
	return nil
}

// RoleUserExists reports whether a user ID exists.
func RoleUserExists(ctx context.Context, q Querier, userID string) (bool, error) {
	var found string
	err := q.QueryRowContext(ctx, `SELECT id FROM users WHERE id=?`, userID).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: find role user: %w", err)
	}
	return true, nil
}
