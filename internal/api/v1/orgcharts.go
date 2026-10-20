package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) listOrganizationCharts(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listOrganizationCharts"))
}
func (a *API) createOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: createOrganizationChart"))
}
func (a *API) deleteOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: deleteOrganizationChart"))
}
func (a *API) getOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getOrganizationChart"))
}
func (a *API) updateOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: updateOrganizationChart"))
}
func (a *API) getCurrentOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getCurrentOrganizationChart"))
}
func (a *API) activateOrganizationChart(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: activateOrganizationChart"))
}
