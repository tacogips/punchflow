package admin

import (
	"errors"
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionIntegration); !ok {
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	items, total, err := store.AdminListTokens(r.Context(), a.d.DB.Q(), "", page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, map[string]any{"id": t.ID, "name": t.Name, "description": t.Description, "createdAt": kftime.Format(t.CreatedAt), "lastUsedAt": pf24FormatPtr(t.LastUsedAt)})
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createServiceAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.Name == "" {
		kferr.Write(w, kferr.InvalidParameter("name is required"))
		return
	}
	var token, id string
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var e error
		token, id, e = a.d.Auth.IssueToken(r.Context(), tx, model.TokenKindSAT, "", body.Name, body.Description)
		if e != nil {
			return e
		}
		return pf24Audit(r.Context(), tx, p, "serviceAccount.create", map[string]string{"serviceAccountId": id})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	t, err := store.TokenByHash(r.Context(), a.d.DB.Q(), auth.HashToken(token))
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": t.ID, "name": t.Name, "description": t.Description, "createdAt": kftime.Format(t.CreatedAt), "lastUsedAt": pf24FormatPtr(t.LastUsedAt), "token": token})
}
func (a *API) deleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	id := r.PathValue("id")
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.AdminDeleteServiceAccount(r.Context(), tx, id); e != nil {
			if errors.Is(e, store.ErrNotFound) {
				return kferr.ResourceNotFound()
			}
			return e
		}
		return pf24Audit(r.Context(), tx, p, "serviceAccount.delete", map[string]string{"serviceAccountId": id})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
