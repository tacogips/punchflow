package v1

import (
	"context"
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

func (a *API) listCategories(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	sort := httpx.ParseSort(r, "name", "createdAt", "updatedAt")
	sortBy := sort.Field
	if sortBy == "" {
		sortBy = "name"
	}
	items, total, err := store.CategoryList(r.Context(), a.d.DB.Q(), p.Offset(), p.PerPage, sortBy, sort.Desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]present.CategoryJSON, 0, len(items))
	for _, item := range items {
		out = append(out, present.Category(item))
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len([]rune(body.Name)) > 100 {
		kferr.Write(w, kferr.InvalidParameter("Invalid name"))
		return
	}
	now := time.Now().UTC()
	id := store.NewID()
	x := model.Category{ID: id, Code: "category-" + id[:8], Name: body.Name, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateCategory(r.Context(), a.d.DB.Q(), x); err != nil {
		if pf20IsUniqueViolation(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Category(x))
}
func (a *API) deleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "categories", r.PathValue("categoryId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	used, err := pf20CategoryUsed(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if used {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"categoryId": {"is used by a workflow"}}))
		return
	}
	if err = store.DeleteCategory(r.Context(), a.d.DB.Q(), id); err != nil {
		if err == store.ErrNotFound {
			kferr.Write(w, kferr.ResourceNotFound())
		} else {
			kferr.Write(w, err)
		}
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) updateCategory(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflowMisc); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "categories", r.PathValue("categoryId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err = httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len([]rune(body.Name)) > 100 {
		kferr.Write(w, kferr.InvalidParameter("Invalid name"))
		return
	}
	if err = store.UpdateCategory(r.Context(), a.d.DB.Q(), id, body.Name, time.Now().UTC().UnixMilli()); err != nil {
		kferr.Write(w, err)
		return
	}
	x, err := store.GetCategory(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Category(x))
}
func pf20CategoryUsed(ctx context.Context, q store.Querier, id string) (bool, error) {
	category, err := store.GetCategory(ctx, q, id)
	if err != nil {
		return false, err
	}
	rows, err := q.QueryContext(ctx, `SELECT definition_json FROM workflows WHERE status!='deleted'`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return false, err
		}
		var d struct {
			CategoryCodes []string `json:"categoryCodes"`
		}
		if err = json.Unmarshal(raw, &d); err != nil {
			return false, err
		}
		for _, code := range d.CategoryCodes {
			if code == category.Code {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}
