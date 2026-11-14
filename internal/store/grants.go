package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"punchflow/internal/model"
)

// UserGrants merges the permission lists of every role assigned to the user
// into one scope per permission. A permission held by any role without a
// restriction is unrestricted; otherwise the targets of all restricted entries
// are unioned. Folder and team targets are expanded to their descendants.
// Permissions that cannot be scoped (see model.PermissionScopeKind) ignore the
// restricted flag.
func UserGrants(ctx context.Context, q Querier, userID string) (map[string]model.PermissionGrant, error) {
	entries, err := userRoleEntries(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	out := map[string]model.PermissionGrant{}
	for _, entry := range entries {
		if entry.Permission == "" {
			continue
		}
		grant := out[entry.Permission]
		kind := model.PermissionScopeKind(entry.Permission)
		if !entry.Restricted || kind == model.ScopeKindNone {
			grant.Unrestricted = true
			out[entry.Permission] = grant
			continue
		}
		if grant.Targets == nil {
			grant.Targets = map[string]bool{}
		}
		var ids []string
		switch kind {
		case model.ScopeKindFolder:
			ids, err = FolderSubtreeIDs(ctx, q, entry.FolderIDs)
		case model.ScopeKindMaster:
			ids = entry.GeneralMasterIDs
		case model.ScopeKindTeam:
			ids, err = TeamSubtreeIDs(ctx, q, entry.TeamIDs)
		}
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			grant.Targets[id] = true
		}
		out[entry.Permission] = grant
	}
	return out, nil
}

func userRoleEntries(ctx context.Context, q Querier, userID string) ([]model.RolePermission, error) {
	rows, err := q.QueryContext(ctx, "SELECT r.permission_list FROM roles r JOIN role_memberships rm ON rm.role_id=r.id WHERE rm.user_id=?", userID)
	if err != nil {
		return nil, fmt.Errorf("store: query user grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []model.RolePermission
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("store: scan grants: %w", err)
		}
		var list []model.RolePermission
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, fmt.Errorf("store: decode grants: %w", err)
		}
		entries = append(entries, list...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read grants: %w", err)
	}
	return entries, nil
}

// FolderSubtreeIDs returns the given folder IDs plus every descendant folder.
func FolderSubtreeIDs(ctx context.Context, q Querier, roots []string) ([]string, error) {
	return subtreeIDs(ctx, q, "folders", roots)
}

// TeamSubtreeIDs returns the given team IDs plus every descendant team.
func TeamSubtreeIDs(ctx context.Context, q Querier, roots []string) ([]string, error) {
	return subtreeIDs(ctx, q, "teams", roots)
}

func subtreeIDs(ctx context.Context, q Querier, table string, roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(roots)), ",")
	args := make([]any, len(roots))
	for i, id := range roots {
		args[i] = id
	}
	query := "WITH RECURSIVE sub(id) AS (SELECT id FROM " + table + " WHERE id IN (" + marks + ") UNION SELECT t.id FROM " + table + " t JOIN sub ON t.parent_id=sub.id) SELECT id FROM sub ORDER BY id"
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: expand %s subtree: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan %s subtree: %w", table, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read %s subtree: %w", table, err)
	}
	return ids, nil
}

// FolderAncestorIDsOrSelf returns the folder ID followed by all its ancestors.
func FolderAncestorIDsOrSelf(ctx context.Context, q Querier, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, "WITH RECURSIVE up(id,parent_id) AS (SELECT id,parent_id FROM folders WHERE id=? UNION SELECT f.id,f.parent_id FROM folders f JOIN up ON f.id=up.parent_id) SELECT id FROM up", id)
	if err != nil {
		return nil, fmt.Errorf("store: folder ancestors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: scan folder ancestors: %w", err)
		}
		ids = append(ids, v)
	}
	return ids, rows.Err()
}

// IDFilter limits a list query to a set of IDs. The zero value applies no
// restriction; a restricted filter with no IDs matches nothing.
type IDFilter struct {
	Restricted bool
	IDs        []string
}

// RestrictedTo returns a filter that admits only the given IDs.
func RestrictedTo(ids []string) IDFilter { return IDFilter{Restricted: true, IDs: ids} }

// clause returns an " AND <column> IN (...)" fragment (empty when unrestricted).
func (f IDFilter) clause(column string) (string, []any) {
	if !f.Restricted {
		return "", nil
	}
	if len(f.IDs) == 0 {
		return " AND 0=1", nil
	}
	args := make([]any, len(f.IDs))
	for i, id := range f.IDs {
		args[i] = id
	}
	return " AND " + column + " IN (" + strings.TrimSuffix(strings.Repeat("?,", len(f.IDs)), ",") + ")", args
}
