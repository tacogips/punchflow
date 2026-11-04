package ticketui

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type ticketLinkBody struct {
	LinkedTicketID string `json:"linkedTicketId"`
}

func (a *API) linkTicket(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	var body ticketLinkBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.LinkedTicketID == "" {
		kferr.Write(w, kferr.InvalidParameter("linkedTicketId is required"))
		return
	}

	p, _ := auth.FromContext(r.Context())
	actor := engine.ActorFromPrincipal(p)
	ticketID := r.PathValue("ticketId")
	linkedTicketID := body.LinkedTicketID
	if err := a.requireLinkAccess(r, actor, ticketID, linkedTicketID); err != nil {
		kferr.Write(w, err)
		return
	}
	if ticketID == linkedTicketID {
		kferr.Write(w, kferr.InvalidParameter("a ticket cannot be linked to itself"))
		return
	}

	if err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.EngLinkTickets(r.Context(), tx, ticketID, linkedTicketID, store.Millis(a.d.Clock.Now())); err != nil {
			return err
		}
		return a.recordTicketLinkAudit(r, tx, p.User.ID, "ticket.link.create", ticketID)
	}); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) unlinkTicket(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	actor := engine.ActorFromPrincipal(p)
	ticketID := r.PathValue("ticketId")
	linkedTicketID := r.PathValue("linkedTicketId")
	if linkedTicketID == "" {
		kferr.Write(w, kferr.InvalidParameter("linkedTicketId is required"))
		return
	}
	if err := a.requireLinkAccess(r, actor, ticketID, linkedTicketID); err != nil {
		kferr.Write(w, err)
		return
	}
	if ticketID == linkedTicketID {
		kferr.Write(w, kferr.InvalidParameter("a ticket cannot be linked to itself"))
		return
	}

	if err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.EngUnlinkTickets(r.Context(), tx, ticketID, linkedTicketID); err != nil {
			return err
		}
		return a.recordTicketLinkAudit(r, tx, p.User.ID, "ticket.link.delete", ticketID)
	}); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) requireLinkAccess(r *http.Request, actor engine.Actor, ticketID, linkedTicketID string) error {
	if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, ticketID); err != nil {
		return err
	}
	return a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, linkedTicketID)
}

func (a *API) recordTicketLinkAudit(r *http.Request, tx *store.Tx, userID, action, ticketID string) error {
	return store.InsertAuditLog(r.Context(), tx, model.AuditLog{
		UserID:    &userID,
		Action:    action,
		TicketID:  &ticketID,
		CreatedAt: a.d.Clock.Now(),
		RemoteIP:  remoteIP(r),
	})
}
