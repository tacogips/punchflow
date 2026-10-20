package admin

import "net/http"

func (a *API) listTokens(w http.ResponseWriter, r *http.Request)  { a.unavailable(w, r) }
func (a *API) createToken(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
