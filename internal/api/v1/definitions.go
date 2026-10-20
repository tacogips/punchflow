package v1

import (
	"net/http"
	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listRoutes(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	s := httpx.ParseSort(r, "createdAt", "folderId")
	by := s.Field
	if by == "" {
		by = "createdAt"
	}
	status := r.URL.Query().Get("status")
	if status != "" && status != "visible" && status != "error" {
		kferr.Write(w, kferr.InvalidParameter("Invalid status"))
		return
	}
	folderID := ""
	if raw := r.URL.Query().Get("folderId"); raw != "" {
		folderID, err = store.ResolveID(r.Context(), a.d.DB.Q(), "folders", raw)
		if err != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
	}
	xs, total, err := store.RouteList(r.Context(), a.d.DB.Q(), p.Offset(), p.PerPage, status, folderID, by, s.Desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]present.RouteJSON, 0, len(xs))
	for _, x := range xs {
		j, e := present.Route(r.Context(), a.d.DB.Q(), x.ID)
		if e != nil {
			kferr.Write(w, e)
			return
		}
		out = append(out, j)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getRoute(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "routes", r.PathValue("routeId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	j, err := present.RouteDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) listWorkflows(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	s := httpx.ParseSort(r, "createdAt", "updatedAt", "name", "status")
	by := s.Field
	if by == "" {
		by = "name"
	}
	status := r.URL.Query().Get("status")
	if status != "" && status != "visible" && status != "invisible" {
		kferr.Write(w, kferr.InvalidParameter("Invalid status"))
		return
	}
	xs, total, err := store.WorkflowList(r.Context(), a.d.DB.Q(), p.Offset(), p.PerPage, status, "", "", by, s.Desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]present.WorkflowJSON, 0, len(xs))
	for _, x := range xs {
		j, e := present.Workflow(r.Context(), a.d.DB.Q(), x.ID)
		if e != nil {
			kferr.Write(w, e)
			return
		}
		out = append(out, j)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "workflows", r.PathValue("workflowId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	j, err := present.WorkflowDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
