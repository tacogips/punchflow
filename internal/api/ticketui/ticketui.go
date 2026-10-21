// Package ticketui serves the punchflow-only ticket operations used by the
// web UI under /ui/api/. They cover capabilities that kickflow exposes in its
// web application but not in its public REST API (ticket labels, approval
// cancellation, raising and per-ticket custom steps), so none of them live
// under /v1. They share the /v1 authentication, CSRF and audit conventions.
package ticketui

import (
	"context"
	"net/http"

	"punchflow/internal/access"
	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

// Prefix is the URL namespace owned by this package.
const Prefix = "/ui/api/"

// EventSink queues webhook events inside the operation's transaction.
type EventSink interface {
	Enqueue(context.Context, store.Querier, []model.WebhookEvent) error
	Notify()
}

// Deps are the collaborators of the API.
type Deps struct {
	DB     *store.DB
	Engine *engine.Engine
	Events EventSink
	Clock  kftime.Clock
}

// API holds the ticket UI handlers.
type API struct{ d Deps }

// New builds the API.
func New(d Deps) *API { return &API{d: d} }

// Operation is one route of the API.
type Operation struct {
	Name    string
	Method  string
	Pattern string
	Handle  func(*API, http.ResponseWriter, *http.Request)
}

var operations = [...]Operation{
	{Name: "listLabels", Method: http.MethodGet, Pattern: "/ui/api/labels", Handle: (*API).listLabels},
	{Name: "listSubStatuses", Method: http.MethodGet, Pattern: "/ui/api/sub-statuses", Handle: (*API).listSubStatuses},
	{Name: "getTicketControls", Method: http.MethodGet, Pattern: "/ui/api/tickets/{ticketId}/controls", Handle: (*API).getControls},
	{Name: "attachTicketLabel", Method: http.MethodPost, Pattern: "/ui/api/tickets/{ticketId}/labels", Handle: (*API).attachLabel},
	{Name: "detachTicketLabel", Method: http.MethodDelete, Pattern: "/ui/api/tickets/{ticketId}/labels/{labelId}", Handle: (*API).detachLabel},
	{Name: "cancelTicketApproval", Method: http.MethodPost, Pattern: "/ui/api/tickets/{ticketId}/cancel-approval", Handle: (*API).cancelApproval},
	{Name: "raiseTicket", Method: http.MethodPost, Pattern: "/ui/api/tickets/{ticketId}/raise", Handle: (*API).raise},
	{Name: "addTicketCustomStep", Method: http.MethodPost, Pattern: "/ui/api/tickets/{ticketId}/custom-steps", Handle: (*API).addCustomStep},
	{Name: "removeTicketCustomStep", Method: http.MethodDelete, Pattern: "/ui/api/tickets/{ticketId}/custom-steps/{stepId}", Handle: (*API).removeCustomStep},
}

// Operations returns a copy of the route table.
func Operations() []Operation { return append([]Operation(nil), operations[:]...) }

// Register installs the routes and a JSON not-found handler for the namespace.
func (a *API) Register(mux *http.ServeMux) {
	a.RegisterRoutes(mux)
	mux.HandleFunc(Prefix, func(w http.ResponseWriter, _ *http.Request) { kferr.Write(w, kferr.EndpointNotFound()) })
}

// RegisterRoutes installs only the routes, for a mux whose /ui/api/ not-found
// handler is owned by another package (the server shares one mux with uiapi).
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	for _, op := range operations {
		operation := op
		mux.Handle(operation.Method+" "+operation.Pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			operation.Handle(a, w, r)
		}))
	}
}

func (a *API) listLabels(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	items, err := store.Labels(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Labels(items))
}

func (a *API) listSubStatuses(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	items, err := store.SubStatuses(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.SubStatuses(items))
}

func (a *API) getControls(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	controls, err := a.d.Engine.Controls(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, r.PathValue("ticketId"))
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, controls)
}

func (a *API) attachLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LabelID string `json:"labelId"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	a.act(w, r, "ticket.label_attach", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.AttachLabel(ctx, tx, actor, id, body.LabelID)
	})
}

func (a *API) detachLabel(w http.ResponseWriter, r *http.Request) {
	a.act(w, r, "ticket.label_detach", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.DetachLabel(ctx, tx, actor, id, r.PathValue("labelId"))
	})
}

func (a *API) cancelApproval(w http.ResponseWriter, r *http.Request) {
	a.act(w, r, "ticket.cancel_approval", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.CancelApproval(ctx, tx, actor, id)
	})
}

func (a *API) raise(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To *int `json:"to"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &body); err != nil {
			kferr.Write(w, err)
			return
		}
	}
	a.act(w, r, "ticket.raise", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Raise(ctx, tx, actor, id, body.To)
	})
}

func (a *API) addCustomStep(w http.ResponseWriter, r *http.Request) {
	var body engine.CustomStepInput
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	a.act(w, r, "ticket.add_custom_step", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.AddCustomStep(ctx, tx, actor, id, body)
	})
}

func (a *API) removeCustomStep(w http.ResponseWriter, r *http.Request) {
	a.act(w, r, "ticket.remove_custom_step", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.RemoveCustomStep(ctx, tx, actor, id, r.PathValue("stepId"))
	})
}

// act runs one engine mutation in a transaction, records the audit log,
// queues the webhook events and answers with the caller's view of the ticket.
func (a *API) act(w http.ResponseWriter, r *http.Request, action string, mutate func(context.Context, *store.Tx, engine.Actor, string) ([]model.WebhookEvent, error)) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	id := r.PathValue("ticketId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	caller := &access.Caller{UserID: p.User.ID, TicketRead: p.Permissions[model.PermissionTicketRead]}
	var out present.TicketDetailJSON
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		events, e := mutate(r.Context(), tx, actor, id)
		if e != nil {
			return e
		}
		userID, ticketID := p.User.ID, id
		audit := model.AuditLog{UserID: &userID, Action: action, TicketID: &ticketID, CreatedAt: a.d.Clock.Now(), RemoteIP: remoteIP(r)}
		if e = store.InsertAuditLog(r.Context(), tx, audit); e != nil {
			return e
		}
		if e = a.d.Events.Enqueue(r.Context(), tx, events); e != nil {
			return e
		}
		out, e = present.TicketDetailFor(r.Context(), tx, id, caller)
		return e
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteJSON(w, http.StatusOK, out)
}

func remoteIP(r *http.Request) *string {
	if r.RemoteAddr == "" {
		return nil
	}
	host := r.RemoteAddr
	return &host
}
