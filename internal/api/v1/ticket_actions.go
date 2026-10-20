package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) approveTicket(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: approveTicket"))
}
func (a *API) rejectTicket(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: rejectTicket"))
}
func (a *API) denyTicket(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: denyTicket"))
}
func (a *API) withdrawTicket(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: withdrawTicket"))
}
func (a *API) archiveTicket(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: archiveTicket"))
}
