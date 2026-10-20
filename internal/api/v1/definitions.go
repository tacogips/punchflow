package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) listRoutes(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listRoutes"))
}
func (a *API) getRoute(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getRoute"))
}
func (a *API) listWorkflows(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listWorkflows"))
}
func (a *API) getWorkflow(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getWorkflow"))
}
