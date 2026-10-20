package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) listTicketLinks(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listTicketLinks"))
}
func (a *API) listViewers(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listViewers"))
}
func (a *API) createViewer(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: createViewer"))
}
func (a *API) deleteViewer(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: deleteViewer"))
}
