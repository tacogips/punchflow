package admin

import "net/http"

func (a *API) getTenant(w http.ResponseWriter, r *http.Request)    { a.unavailable(w, r) }
func (a *API) updateTenant(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
