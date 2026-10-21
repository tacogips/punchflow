// Package uiapi serves punchflow's browser-only helper endpoints under
// /ui/api/. They back UI features that kickflow's public REST API does not
// document - ticket watch subscriptions, the local notification inbox, ticket
// keyword search and CSV exports - and are deliberately kept out of /v1 so the
// kickflow-compatible contract stays untouched. Requests are authenticated by
// the same middleware as /v1 (browser session or access token).
package uiapi

import (
	"context"
	"net/http"

	"punchflow/internal/engine"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

// Viewer decides ticket visibility; *engine.Engine implements it.
type Viewer interface {
	CanView(ctx context.Context, q store.Querier, a engine.Actor, ticketID string) error
}

// Deps contains the services used by the handlers.
type Deps struct {
	DB     *store.DB
	Engine Viewer
	// V1 is the /v1 handler. Search and export reuse its ticket list so that
	// filters and effective visibility stay identical to GET /v1/tickets.
	V1      http.Handler
	Clock   kftime.Clock
	BaseURL string
}

// API registers the UI helper routes.
type API struct{ d Deps }

// New constructs the UI API. A nil clock selects the system clock.
func New(d Deps) *API {
	if d.Clock == nil {
		d.Clock = kftime.SystemClock{}
	}
	return &API{d: d}
}

// Operation is one row in the UI API route table.
type Operation struct {
	Name, Method, Pattern string
	Handle                func(a *API, w http.ResponseWriter, r *http.Request)
}

var operations = [...]Operation{
	{Name: "getTicketWatch", Method: http.MethodGet, Pattern: "/ui/api/tickets/{ticketId}/watch", Handle: (*API).getWatch},
	{Name: "watchTicket", Method: http.MethodPost, Pattern: "/ui/api/tickets/{ticketId}/watch", Handle: (*API).watchTicket},
	{Name: "unwatchTicket", Method: http.MethodDelete, Pattern: "/ui/api/tickets/{ticketId}/watch", Handle: (*API).unwatchTicket},
	{Name: "listNotifications", Method: http.MethodGet, Pattern: "/ui/api/notifications", Handle: (*API).listNotifications},
	{Name: "countUnreadNotifications", Method: http.MethodGet, Pattern: "/ui/api/notifications/unread-count", Handle: (*API).unreadCount},
	{Name: "markAllNotificationsRead", Method: http.MethodPost, Pattern: "/ui/api/notifications/read-all", Handle: (*API).markAllRead},
	{Name: "markNotificationRead", Method: http.MethodPost, Pattern: "/ui/api/notifications/{id}/read", Handle: (*API).markRead},
	{Name: "searchTickets", Method: http.MethodGet, Pattern: "/ui/api/tickets/search", Handle: (*API).searchTickets},
	{Name: "exportTickets", Method: http.MethodGet, Pattern: "/ui/api/tickets/export", Handle: (*API).exportTickets},
	{Name: "exportAuditLogs", Method: http.MethodGet, Pattern: "/ui/api/audit-logs/export", Handle: (*API).exportAuditLogs},
}

// Operations returns a copy of the route table.
func Operations() []Operation { return append([]Operation(nil), operations[:]...) }

// Register installs every route and a JSON not-found handler for the namespace.
func (a *API) Register(mux *http.ServeMux) {
	for _, op := range operations {
		operation := op
		mux.Handle(operation.Method+" "+operation.Pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			operation.Handle(a, w, r)
		}))
	}
	mux.HandleFunc("/ui/api/", func(w http.ResponseWriter, _ *http.Request) {
		kferr.Write(w, kferr.EndpointNotFound())
	})
}
