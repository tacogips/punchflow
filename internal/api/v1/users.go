package v1

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) getCurrentUser(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok || p.User.ID == "" {
		kferr.Write(w, kferr.New(http.StatusUnauthorized, "unauthorized", "Unauthorized"))
		return
	}
	x, err := pf18UserDetail(r.Context(), a.d.DB.Q(), p.User.ID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, x)
}
func (a *API) getUser(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if err != nil {
		kferr.Write(w, kferr.New(404, "resource_not_found", "User not found"))
		return
	}
	x, err := pf18UserDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.New(404, "resource_not_found", "User not found"))
		return
	}
	httpx.WriteJSON(w, 200, x)
}
