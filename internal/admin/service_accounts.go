package admin

import "net/http"

func (a *API) listServiceAccounts(w http.ResponseWriter, r *http.Request)  { a.unavailable(w, r) }
func (a *API) createServiceAccount(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
func (a *API) deleteServiceAccount(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
