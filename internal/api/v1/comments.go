package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) listComments(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: listComments"))
}
func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: createComment"))
}
func (a *API) getComment(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getComment"))
}
func (a *API) updateComment(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: updateComment"))
}
func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: deleteComment"))
}
