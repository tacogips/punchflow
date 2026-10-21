package v1

import (
	"context"
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) approveTicket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Comment *string `json:"comment"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	a.pf22Action(w, r, "ticket.approve", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Approve(ctx, tx, actor, id, body.Comment)
	})
}
func (a *API) rejectTicket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To      *int    `json:"to"`
		Comment *string `json:"comment"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.To == nil {
		kferr.Write(w, kferr.InvalidParameter("to is required"))
		return
	}
	a.pf22Action(w, r, "ticket.reject", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.SendBack(ctx, tx, actor, id, *body.To, body.Comment)
	})
}
func (a *API) denyTicket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Comment *string `json:"comment"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	a.pf22Action(w, r, "ticket.deny", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Deny(ctx, tx, actor, id, body.Comment)
	})
}
func (a *API) withdrawTicket(w http.ResponseWriter, r *http.Request) {
	a.pf22Action(w, r, "ticket.withdraw", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Withdraw(ctx, tx, actor, id)
	})
}
func (a *API) archiveTicket(w http.ResponseWriter, r *http.Request) {
	a.pf22Action(w, r, "ticket.archive", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Archive(ctx, tx, actor, id)
	})
}
func (a *API) pf22Action(w http.ResponseWriter, r *http.Request, action string, mutate func(context.Context, *store.Tx, engine.Actor, string) ([]model.WebhookEvent, error)) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	id := r.PathValue("ticketId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	var out present.TicketDetailJSON
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		events, e := mutate(r.Context(), tx, actor, id)
		if e != nil {
			return e
		}
		if e = store.InsertAuditLog(r.Context(), tx, pf22Audit(p.User.ID, action, id, r)); e != nil {
			return e
		}
		if e = a.d.Events.Enqueue(r.Context(), tx, events); e != nil {
			return e
		}
		out, e = present.TicketDetailFor(r.Context(), tx, id, pf16Caller(p.User.ID, p.Permissions))
		return e
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteJSON(w, http.StatusOK, out)
}
