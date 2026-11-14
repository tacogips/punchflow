package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, "")
	if !ok {
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	items, total, err := store.AdminListTokens(r.Context(), a.d.DB.Q(), p.User.ID, page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		out = append(out, pf24TokenJSON(t))
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createToken(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, "")
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
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
		token, id, e = a.d.Auth.IssueToken(r.Context(), tx, model.TokenKindPAT, p.User.ID, body.Name, "")
		if e != nil {
			return e
		}
		return pf24Audit(r.Context(), tx, p, "token.create", map[string]string{"tokenId": id})
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
	out := pf24TokenJSON(t)
	out["token"] = token
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, "")
	if !ok {
		return
	}
	id := r.PathValue("tokenId")
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.AdminDeleteToken(r.Context(), tx, id, p.User.ID, p.Has(model.PermissionIntegration)); e != nil {
			if errors.Is(e, store.ErrNotFound) {
				return kferr.ResourceNotFound()
			}
			return e
		}
		return pf24Audit(r.Context(), tx, p, "token.delete", map[string]string{"tokenId": id})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
func pf24Principal(w http.ResponseWriter, r *http.Request, permission string) (*auth.Principal, bool) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return nil, false
	}
	if permission != "" && !p.Has(permission) {
		kferr.Write(w, kferr.MissingPermission("Missing permission"))
		return nil, false
	}
	return p, true
}
func pf24TokenJSON(t model.APIToken) map[string]any {
	return map[string]any{"id": t.ID, "name": t.Name, "createdAt": kftime.Format(t.CreatedAt), "lastUsedAt": pf24FormatPtr(t.LastUsedAt)}
}
func pf24FormatPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return kftime.Format(*t)
}
func pf24Audit(ctx context.Context, q store.Querier, p *auth.Principal, action string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	uid := p.User.ID
	return store.InsertAuditLog(ctx, q, model.AuditLog{ID: store.NewID(), UserID: &uid, Action: action, Data: raw})
}
