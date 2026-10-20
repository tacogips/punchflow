package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: uploadFile"))
}
func (a *API) getFile(w http.ResponseWriter, r *http.Request) {
	kferr.Write(w, kferr.New(http.StatusInternalServerError, "internal_server_error", "not implemented: getFile"))
}
