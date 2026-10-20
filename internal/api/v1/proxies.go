package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) listProxyApplicants(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listProxyApplicants"))
}
func (a *API) createProxyApplicant(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: createProxyApplicant"))
}
func (a *API) updateProxyApplicant(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: updateProxyApplicant"))
}
func (a *API) deleteProxyApplicant(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: deleteProxyApplicant"))
}
func (a *API) listProxyApprovers(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listProxyApprovers"))
}
func (a *API) createProxyApprover(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: createProxyApprover"))
}
func (a *API) updateProxyApprover(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: updateProxyApprover"))
}
func (a *API) deleteProxyApprover(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: deleteProxyApprover"))
}
