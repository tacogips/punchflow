package uiapi

import (
	"context"
	"net/http"
	"strings"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

type actorJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type notificationJSON struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	TicketID      string     `json:"ticketId"`
	TicketNumber  *string    `json:"ticketNumber"`
	TicketTitle   *string    `json:"ticketTitle"`
	TicketVisible bool       `json:"ticketVisible"`
	CommentID     *string    `json:"commentId"`
	Actor         *actorJSON `json:"actor"`
	Excerpt       string     `json:"excerpt"`
	Read          bool       `json:"read"`
	ReadAt        *string    `json:"readAt"`
	CreatedAt     string     `json:"createdAt"`
}

func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	unread := r.URL.Query().Get("unread")
	if unread != "" && unread != "true" && unread != "false" {
		kferr.Write(w, kferr.InvalidParameter("Invalid unread"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	items, total, err := store.ListNotifications(r.Context(), a.d.DB.Q(), p.User.ID, unread == "true", page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	actor := engine.ActorFromPrincipal(p)
	out := make([]notificationJSON, 0, len(items))
	for _, item := range items {
		entry, err := a.presentNotification(r.Context(), actor, item)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		out = append(out, entry)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}

// presentNotification joins ticket and actor details. The ticket title and
// number are withheld when the user can no longer view the ticket.
func (a *API) presentNotification(ctx context.Context, viewer engine.Actor, n store.Notification) (notificationJSON, error) {
	q := a.d.DB.Q()
	out := notificationJSON{ID: n.ID, Kind: n.Kind, TicketID: n.TicketID, CommentID: n.CommentID, Excerpt: n.Excerpt, Read: n.ReadAt != nil, CreatedAt: kftime.Format(n.CreatedAt)}
	if n.ReadAt != nil {
		readAt := kftime.Format(*n.ReadAt)
		out.ReadAt = &readAt
	}
	if a.d.Engine.CanView(ctx, q, viewer, n.TicketID) == nil {
		ticket, err := store.EngLoadTicket(ctx, q, n.TicketID)
		if err != nil {
			return out, err
		}
		out.TicketVisible, out.TicketNumber, out.TicketTitle = true, ticket.TicketNumber, ticket.Title
	}
	if n.ActorUserID != nil {
		if user, err := store.GetUser(ctx, q, *n.ActorUserID); err == nil {
			out.Actor = &actorJSON{ID: user.ID, Name: strings.TrimSpace(user.LastName + " " + user.FirstName)}
		}
	}
	return out, nil
}

func (a *API) unreadCount(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	count, err := store.CountUnreadNotifications(r.Context(), a.d.DB.Q(), p.User.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (a *API) markRead(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	if err := store.MarkNotificationRead(r.Context(), a.d.DB.Q(), p.User.ID, r.PathValue("id"), a.d.Clock.Now()); err != nil {
		if err == store.ErrNotFound {
			err = kferr.ResourceNotFound()
		}
		kferr.Write(w, err)
		return
	}
	a.unreadCount(w, r)
}

func (a *API) markAllRead(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	if _, err := store.MarkAllNotificationsRead(r.Context(), a.d.DB.Q(), p.User.ID, a.d.Clock.Now()); err != nil {
		kferr.Write(w, err)
		return
	}
	a.unreadCount(w, r)
}
