package v1

import (
	"encoding/json"
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

func (a *API) listFolders(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	s := httpx.ParseSort(r, "createdAt", "name", "fullName")
	by := s.Field
	if by == "" {
		by = "name"
	}
	xs, total, err := store.FolderList(r.Context(), a.d.DB.Q(), p.Offset(), p.PerPage, by, s.Desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]present.FolderJSON, 0, len(xs))
	for _, x := range xs {
		j, e := present.Folder(r.Context(), a.d.DB.Q(), x.ID)
		if e != nil {
			kferr.Write(w, e)
			return
		}
		out = append(out, j)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createFolder(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	var b struct {
		Name        string  `json:"name"`
		Code        string  `json:"code"`
		Description *string `json:"description"`
		Parent      *string `json:"parentFolderId"`
	}
	if err := httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len([]rune(b.Name)) > 300 {
		kferr.Write(w, kferr.InvalidParameter("Invalid name"))
		return
	}
	if b.Code == "" {
		id := store.NewID()
		b.Code = "folder-" + id[:8]
	}
	if len([]rune(b.Code)) > 100 {
		kferr.Write(w, kferr.InvalidParameter("Invalid code"))
		return
	}
	// An omitted or null parentFolderId places the folder directly under the root folder.
	var parent string
	if b.Parent == nil {
		root, err := store.RootFolderID(r.Context(), a.d.DB.Q())
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		parent = root
	} else {
		resolved, err := store.ResolveID(r.Context(), a.d.DB.Q(), "folders", *b.Parent)
		if err != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		parent = resolved
	}
	now := time.Now().UTC()
	x := model.Folder{ID: store.NewID(), Code: b.Code, Name: b.Name, Description: b.Description, ParentID: &parent, Editable: true, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateFolder(r.Context(), a.d.DB.Q(), x); err != nil {
		if pf20IsUniqueViolation(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	j, err := present.FolderDetail(r.Context(), a.d.DB.Q(), x.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) deleteFolder(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "folders", r.PathValue("folderId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	x, err := store.GetFolder(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if !x.Editable {
		kferr.Write(w, kferr.ResourceNotEditable())
		return
	}
	p := principalOf(r)
	now := a.d.Clock.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		tree, e := store.FolderSubtreeIDs(r.Context(), tx, []string{id})
		if e != nil {
			return e
		}
		// Deleting a folder also retires the workflows and routes under it, which
		// needs the matching permission for each affected folder.
		workflowFolders, routeFolders, e := store.FolderTreeDefinitionFolders(r.Context(), tx, tree)
		if e != nil {
			return e
		}
		for _, folder := range workflowFolders {
			if e = requireTarget(p, folder, model.PermissionWorkflow); e != nil {
				return e
			}
		}
		for _, folder := range routeFolders {
			if e = requireTarget(p, folder, model.PermissionRoute); e != nil {
				return e
			}
		}
		if e = store.RemoveFoldersFromRoleTargets(r.Context(), tx, tree, now); e != nil {
			return e
		}
		_, e = store.DeleteFolderTree(r.Context(), tx, id, now)
		return e
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) updateFolder(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "folders", r.PathValue("folderId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	x, err := store.GetFolder(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if !x.Editable {
		kferr.Write(w, kferr.ResourceNotEditable())
		return
	}
	var b map[string]json.RawMessage
	if err = httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	if len(b) == 0 {
		kferr.Write(w, kferr.InvalidParameter("At least one field is required"))
		return
	}
	if raw, ok := b["name"]; ok {
		if err = json.Unmarshal(raw, &x.Name); err != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid name"))
			return
		}
		x.Name = strings.TrimSpace(x.Name)
		if x.Name == "" || len([]rune(x.Name)) > 300 {
			kferr.Write(w, kferr.InvalidParameter("Invalid name"))
			return
		}
	}
	if raw, ok := b["code"]; ok {
		if err = json.Unmarshal(raw, &x.Code); err != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid code"))
			return
		}
		if strings.TrimSpace(x.Code) == "" || len([]rune(x.Code)) > 100 {
			kferr.Write(w, kferr.InvalidParameter("Invalid code"))
			return
		}
	}
	if raw, ok := b["description"]; ok {
		if string(raw) == "null" {
			x.Description = nil
		} else {
			var v string
			if err = json.Unmarshal(raw, &v); err != nil {
				kferr.Write(w, kferr.InvalidParameter("Invalid description"))
				return
			}
			x.Description = &v
		}
	}
	if raw, ok := b["parentFolderId"]; ok {
		if string(raw) == "null" {
			root, e := store.RootFolderID(r.Context(), a.d.DB.Q())
			if e != nil {
				kferr.Write(w, kferr.Internal())
				return
			}
			x.ParentID = &root
		} else {
			var v string
			if err = json.Unmarshal(raw, &v); err != nil {
				kferr.Write(w, kferr.InvalidParameter("Invalid parentFolderId"))
				return
			}
			pid, e := store.ResolveID(r.Context(), a.d.DB.Q(), "folders", v)
			if e != nil {
				kferr.Write(w, kferr.ResourceNotFound())
				return
			}
			if pid == id {
				kferr.Write(w, kferr.ValidationFailed(map[string][]string{"parentFolderId": {"would create a folder cycle"}}))
				return
			}
			anc, e := store.GetFolderAncestors(r.Context(), a.d.DB.Q(), pid)
			if e != nil {
				kferr.Write(w, e)
				return
			}
			for _, a := range anc {
				if a.ID == id {
					kferr.Write(w, kferr.ValidationFailed(map[string][]string{"parentFolderId": {"would create a folder cycle"}}))
					return
				}
			}
			x.ParentID = &pid
		}
	}
	x.UpdatedAt = time.Now().UTC()
	if err = store.UpdateFolder(r.Context(), a.d.DB.Q(), x); err != nil {
		if pf20IsUniqueViolation(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	j, err := present.FolderDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) getFolder(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "folders", r.PathValue("folderId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	j, err := present.FolderDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
