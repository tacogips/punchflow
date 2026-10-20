package v1

import (
	"encoding/json"
	"net/http"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf19ChartBody struct {
	Name string `json:"name"`
}

func (a *API) listOrganizationCharts(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	xs, n, err := store.OrgChartList(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out := make([]any, 0, len(xs))
	for _, x := range xs {
		v, e := present.OrganizationChart(r.Context(), a.d.DB.Q(), x.ID)
		if e != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		out = append(out, v)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, n)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	var b pf19ChartBody
	if err := httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.Name == "" || len(b.Name) > 300 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"name": {"is required and must be at most 300 characters"}}))
		return
	}
	p, _ := auth.FromContext(r.Context())
	now := time.Now()
	x := model.OrganizationChart{ID: store.NewID(), Name: b.Name, CreatedAt: now, UpdatedAt: now}
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.OrgChartInsert(r.Context(), tx, x); e != nil {
			return e
		}
		return a.pf19ChartAudit(r, tx, p.User.ID, "organizationChart.create", x.ID)
	})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, err := present.OrganizationChart(r.Context(), a.d.DB.Q(), x.ID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("organizationChartId")
	if _, err := store.GetOrganizationChart(r.Context(), a.d.DB.Q(), id); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	out, err := present.OrganizationChart(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getCurrentOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ActiveOrgChartID(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	out, err := present.OrganizationChart(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) updateOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("organizationChartId")
	if _, err := store.GetOrganizationChart(r.Context(), a.d.DB.Q(), id); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	var b pf19ChartBody
	if err := httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.Name == "" || len(b.Name) > 300 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"name": {"is required and must be at most 300 characters"}}))
		return
	}
	p, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.OrgChartUpdate(r.Context(), tx, id, b.Name, time.Now()); e != nil {
			return e
		}
		return a.pf19ChartAudit(r, tx, p.User.ID, "organizationChart.update", id)
	})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, err := present.OrganizationChart(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) activateOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("organizationChartId")
	if _, err := store.GetOrganizationChart(r.Context(), a.d.DB.Q(), id); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	p, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.OrgChartActivate(r.Context(), tx, id, time.Now()); e != nil {
			return e
		}
		return a.pf19ChartAudit(r, tx, p.User.ID, "organizationChart.activate", id)
	})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, err := present.OrganizationChartDetail(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) deleteOrganizationChart(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	id := r.PathValue("organizationChartId")
	x, err := store.GetOrganizationChart(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if x.Current {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"base": {"current organization chart cannot be deleted"}}))
		return
	}
	p, _ := auth.FromContext(r.Context())
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.OrgChartDelete(r.Context(), tx, id); e != nil {
			return e
		}
		return a.pf19ChartAudit(r, tx, p.User.ID, "organizationChart.delete", id)
	})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) pf19ChartAudit(r *http.Request, q store.Querier, user, action, id string) error {
	data, _ := json.Marshal(map[string]string{"organizationChartId": id})
	ip := httpx.ClientIP(r)
	rid := httpx.RequestIDFrom(r.Context())
	system := "api"
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &user, Action: action, Data: data, RemoteIP: &ip, RequestID: &rid, SystemType: &system, CreatedAt: time.Now()})
}
