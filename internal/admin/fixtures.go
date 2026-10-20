package admin

import "net/http"

func (a *API) exportFixtures(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
func (a *API) resetFixtures(w http.ResponseWriter, r *http.Request)  { a.unavailable(w, r) }
