package present

import (
	"context"
	"punchflow/internal/store"
)

func Role(ctx context.Context, q store.Querier, id string) (RoleJSON, error) {
	m, e := store.GetRole(ctx, q, id)
	if e != nil {
		return nil, e
	}
	n, e := store.CountRoleUsers(ctx, q, id)
	if e != nil {
		return nil, e
	}
	return RoleJSON{"id": m.ID, "name": m.Name, "editable": m.Editable, "usersCount": n, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}, nil
}
func RoleDetail(ctx context.Context, q store.Querier, id string) (RoleDetailJSON, error) {
	m, e := store.GetRole(ctx, q, id)
	if e != nil {
		return nil, e
	}
	base, e := Role(ctx, q, id)
	if e != nil {
		return nil, e
	}
	perms := []any{}
	for _, p := range m.PermissionList {
		perms = append(perms, map[string]any{"permission": p.Permission, "restricted": false, "folders": []any{}, "generalMasters": []any{}, "teams": []any{}})
	}
	out := RoleDetailJSON(base)
	out["permissionList"] = perms
	return out, nil
}
