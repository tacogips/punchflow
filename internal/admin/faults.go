package admin

import "net/http"

func (a *API) listFaults(w http.ResponseWriter, r *http.Request)  { a.unavailable(w, r) }
func (a *API) createFault(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
func (a *API) deleteFault(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
