package admin

import "net/http"

func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request)       { a.unavailable(w, r) }
func (a *API) createWebhook(w http.ResponseWriter, r *http.Request)      { a.unavailable(w, r) }
func (a *API) getWebhook(w http.ResponseWriter, r *http.Request)         { a.unavailable(w, r) }
func (a *API) updateWebhook(w http.ResponseWriter, r *http.Request)      { a.unavailable(w, r) }
func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request)      { a.unavailable(w, r) }
func (a *API) pingWebhook(w http.ResponseWriter, r *http.Request)        { a.unavailable(w, r) }
func (a *API) listWebhookEvents(w http.ResponseWriter, r *http.Request)  { a.unavailable(w, r) }
func (a *API) getWebhookEvent(w http.ResponseWriter, r *http.Request)    { a.unavailable(w, r) }
func (a *API) resendWebhookEvent(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
func (a *API) createWebhookEvent(w http.ResponseWriter, r *http.Request) { a.unavailable(w, r) }
