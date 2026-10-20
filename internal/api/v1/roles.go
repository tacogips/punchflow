package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf21RolePermissionBody struct {
	Permission       string   `json:"permission"`
	Restricted       *bool    `json:"restricted"`
	FolderIDs        []string `json:"folderIds"`
	GeneralMasterIDs []string `json:"generalMasterIds"`
	TeamIDs          []string `json:"teamIds"`
}

type pf21RoleCreateBody struct {
	Name           string                   `json:"name"`
	PermissionList []pf21RolePermissionBody `json:"permissionList"`
}

type pf21RoleUpdateBody struct {
	Name           *string                   `json:"name"`
	PermissionList *[]pf21RolePermissionBody `json:"permissionList"`
}

func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	sort := httpx.ParseSort(r, "name", "createdAt")
	ids, total, err := store.RoleList(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage, sort.Field, sort.Desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	items := make([]present.RoleJSON, 0, len(ids))
	for _, id := range ids {
		item, err := present.Role(r.Context(), a.d.DB.Q(), id)
		if err != nil {
			kferr.Write(w, pf21RoleError(err))
			return
		}
		items = append(items, item)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *API) createRole(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	var body pf21RoleCreateBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.PermissionList == nil {
		kferr.Write(w, kferr.InvalidParameter("Missing name or permissionList"))
		return
	}
	permissions, modeled, err := pf21ValidatePermissions(r.Context(), a.d.DB.Q(), body.PermissionList)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	principal, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	role := model.Role{ID: store.NewID(), Name: body.Name, Editable: true, PermissionList: modeled, CreatedAt: now, UpdatedAt: now}
	if err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.RoleCreate(r.Context(), tx, role, permissions); err != nil {
			return err
		}
		return pf21RecordRoleAudit(r, tx, principal.User.ID, "role.create", role.ID, []string{"name", "permissionList"})
	}); err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	out, err := pf21RoleDetail(r.Context(), a.d.DB.Q(), role.ID)
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) getRole(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("roleId")
	if !store.IsUUID(id) {
		kferr.Write(w, kferr.InvalidParameter("Invalid roleId"))
		return
	}
	out, err := pf21RoleDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) updateRole(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("roleId")
	if !store.IsUUID(id) {
		kferr.Write(w, kferr.InvalidParameter("Invalid roleId"))
		return
	}
	var body pf21RoleUpdateBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.Name == nil && body.PermissionList == nil {
		kferr.Write(w, kferr.InvalidParameter("Empty update"))
		return
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) == "" {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"name": {"must not be blank"}}))
		return
	}
	var permissions json.RawMessage
	var modeled []model.RolePermission
	var err error
	if body.PermissionList != nil {
		permissions, modeled, err = pf21ValidatePermissions(r.Context(), a.d.DB.Q(), *body.PermissionList)
		if err != nil {
			kferr.Write(w, err)
			return
		}
	}
	principal, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	if err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		role, err := store.GetRole(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if !role.Editable {
			return kferr.ResourceNotEditable()
		}
		member, err := store.RoleHasMember(r.Context(), tx, id, principal.User.ID)
		if err != nil {
			return err
		}
		superAdmin, err := store.RoleIsSuperAdminMember(r.Context(), tx, principal.User.ID)
		if err != nil {
			return err
		}
		if member && !superAdmin {
			return kferr.MissingPermission("Cannot update a role you belong to")
		}
		if err := store.RoleUpdate(r.Context(), tx, id, body.Name, permissions, store.Millis(now)); err != nil {
			return err
		}
		fields := []string{}
		if body.Name != nil {
			fields = append(fields, "name")
		}
		if body.PermissionList != nil {
			fields = append(fields, "permissionList")
		}
		return pf21RecordRoleAudit(r, tx, principal.User.ID, "role.update", id, fields)
	}); err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	_ = modeled
	out, err := pf21RoleDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) deleteRole(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("roleId")
	if !store.IsUUID(id) {
		kferr.Write(w, kferr.InvalidParameter("Invalid roleId"))
		return
	}
	principal, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		role, err := store.GetRole(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if !role.Editable {
			return kferr.ResourceNotEditable()
		}
		member, err := store.RoleHasMember(r.Context(), tx, id, principal.User.ID)
		if err != nil {
			return err
		}
		superAdmin, err := store.RoleIsSuperAdminMember(r.Context(), tx, principal.User.ID)
		if err != nil {
			return err
		}
		if member && !superAdmin {
			return kferr.MissingPermission("Cannot delete a role you belong to")
		}
		if err := store.RoleDelete(r.Context(), tx, id); err != nil {
			return err
		}
		return pf21RecordRoleAudit(r, tx, principal.User.ID, "role.delete", id, []string{"id"})
	})
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteEmpty(w)
}

func (a *API) createRoleMembers(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	roleID := r.PathValue("roleId")
	if !store.IsUUID(roleID) {
		kferr.Write(w, kferr.InvalidParameter("Invalid roleId"))
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if len(body.UserIDs) == 0 || len(body.UserIDs) > 10 {
		kferr.Write(w, kferr.InvalidParameter("userIds must contain between 1 and 10 values"))
		return
	}
	principal, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if _, err := store.GetRole(r.Context(), tx, roleID); err != nil {
			return err
		}
		superAdmin, err := store.RoleIsSuperAdminMember(r.Context(), tx, principal.User.ID)
		if err != nil {
			return err
		}
		for _, userID := range body.UserIDs {
			if !store.IsUUID(userID) {
				return kferr.InvalidParameter("Invalid userId")
			}
			found, err := store.RoleUserExists(r.Context(), tx, userID)
			if err != nil {
				return err
			}
			if !found {
				return kferr.ResourceNotFound()
			}
			if userID == principal.User.ID && !superAdmin {
				return kferr.MissingPermission("Cannot add yourself to a role")
			}
		}
		if err := store.RoleAddMembers(r.Context(), tx, roleID, body.UserIDs, store.Millis(a.d.Clock.Now())); err != nil {
			return err
		}
		return pf21RecordRoleAudit(r, tx, principal.User.ID, "role.members.create", roleID, []string{"userIds"})
	})
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteEmpty(w)
}

func (a *API) listRoleMembers(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	roleID := r.PathValue("roleId")
	if !store.IsUUID(roleID) {
		kferr.Write(w, kferr.InvalidParameter("Invalid roleId"))
		return
	}
	if _, err := store.GetRole(r.Context(), a.d.DB.Q(), roleID); err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	ids, total, err := store.RoleMemberIDs(r.Context(), a.d.DB.Q(), roleID, page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	items, err := present.Users(r.Context(), a.d.DB.Q(), ids, present.UserOpts{})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *API) deleteRoleMember(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	roleID, userID := r.PathValue("roleId"), r.PathValue("userId")
	if !store.IsUUID(roleID) || !store.IsUUID(userID) {
		kferr.Write(w, kferr.InvalidParameter("Invalid role or user id"))
		return
	}
	principal, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		role, err := store.GetRole(r.Context(), tx, roleID)
		if err != nil {
			return err
		}
		member, err := store.RoleHasMember(r.Context(), tx, roleID, userID)
		if err != nil {
			return err
		}
		if !member {
			return store.ErrNotFound
		}
		superAdmin, err := store.RoleIsSuperAdminMember(r.Context(), tx, principal.User.ID)
		if err != nil {
			return err
		}
		if userID == principal.User.ID && !superAdmin {
			return kferr.MissingPermission("Cannot remove yourself from a role")
		}
		if !role.Editable {
			ids, _, err := store.RoleMemberIDs(r.Context(), tx, roleID, 0, 2)
			if err != nil {
				return err
			}
			if len(ids) <= 1 {
				return kferr.ValidationFailed(map[string][]string{"userIds": {"must retain a super admin member"}})
			}
		}
		if err := store.RoleRemoveMember(r.Context(), tx, roleID, userID); err != nil {
			return err
		}
		return pf21RecordRoleAudit(r, tx, principal.User.ID, "role.members.delete", roleID, []string{"userId"})
	})
	if err != nil {
		kferr.Write(w, pf21RoleError(err))
		return
	}
	httpx.WriteEmpty(w)
}

func pf21ValidatePermissions(ctx context.Context, q store.Querier, entries []pf21RolePermissionBody) (json.RawMessage, []model.RolePermission, error) {
	allowed := make(map[string]bool, len(model.Permissions))
	for _, value := range model.Permissions {
		allowed[value] = true
	}
	modeled := make([]model.RolePermission, 0, len(entries))
	for _, entry := range entries {
		if !allowed[entry.Permission] {
			return nil, nil, kferr.InvalidParameter("Invalid permission")
		}
		if entry.Restricted == nil {
			return nil, nil, kferr.InvalidParameter("Missing restricted")
		}
		if err := pf21CheckTargetIDs(ctx, q, "folders", entry.FolderIDs); err != nil {
			return nil, nil, err
		}
		if err := pf21CheckTargetIDs(ctx, q, "general_masters", entry.GeneralMasterIDs); err != nil {
			return nil, nil, err
		}
		if err := pf21CheckTargetIDs(ctx, q, "teams", entry.TeamIDs); err != nil {
			return nil, nil, err
		}
		modeled = append(modeled, model.RolePermission{Permission: entry.Permission})
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, kferr.InvalidParameter("Invalid permissionList")
	}
	return raw, modeled, nil
}

func pf21CheckTargetIDs(ctx context.Context, q store.Querier, table string, ids []string) error {
	seen := map[string]string{}
	for _, id := range ids {
		if !store.IsUUID(id) {
			return kferr.InvalidParameter("Invalid target id")
		}
		folded := strings.ToLower(id)
		if previous, ok := seen[folded]; ok {
			if previous == id {
				return kferr.InvalidParameter("Duplicate target id")
			}
			return kferr.ValidationFailed(map[string][]string{"permissionList": {"contains duplicate target IDs"}})
		}
		seen[folded] = id
		var found string
		err := q.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE id=?", id).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return kferr.ValidationFailed(map[string][]string{"permissionList": {"contains an unknown target ID"}})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func pf21RoleDetail(ctx context.Context, q store.Querier, id string) (present.RoleDetailJSON, error) {
	out, err := present.RoleDetail(ctx, q, id)
	if err != nil {
		return nil, err
	}
	raw, err := store.RolePermissionJSON(ctx, q, id)
	if err != nil {
		return nil, err
	}
	var entries []pf21RolePermissionBody
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	permissions := make([]any, 0, len(entries))
	for _, entry := range entries {
		folders := make([]any, 0, len(entry.FolderIDs))
		for _, id := range entry.FolderIDs {
			item, err := present.Folder(ctx, q, id)
			if err != nil {
				return nil, err
			}
			folders = append(folders, item)
		}
		masters := make([]any, 0, len(entry.GeneralMasterIDs))
		for _, id := range entry.GeneralMasterIDs {
			item, err := present.GeneralMaster(ctx, q, id)
			if err != nil {
				return nil, err
			}
			masters = append(masters, item)
		}
		teams := make([]any, 0, len(entry.TeamIDs))
		for _, id := range entry.TeamIDs {
			item, err := present.Team(ctx, q, id)
			if err != nil {
				return nil, err
			}
			teams = append(teams, item)
		}
		permissions = append(permissions, map[string]any{"permission": entry.Permission, "restricted": entry.Restricted != nil && *entry.Restricted, "folders": folders, "generalMasters": masters, "teams": teams})
	}
	out["permissionList"] = permissions
	return out, nil
}

func pf21RecordRoleAudit(r *http.Request, q store.Querier, userID, action, roleID string, fields []string) error {
	data, err := json.Marshal(map[string]any{"id": roleID, "fields": fields})
	if err != nil {
		return err
	}
	remoteIP, requestID := httpx.ClientIP(r), httpx.RequestIDFrom(r.Context())
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &userID, Action: action, Data: data, RemoteIP: &remoteIP, RequestID: &requestID, CreatedAt: time.Time{}})
}

func pf21RoleError(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return kferr.ResourceNotFound()
	}
	return err
}
