package v1

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listComments(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	ticketID := r.PathValue("ticketId")
	if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	sort := httpx.ParseSort(r, "createdAt")
	direction := "ASC"
	if sort.Set && sort.Desc {
		direction = "DESC"
	}
	var total int
	if err = a.d.DB.Q().QueryRowContext(r.Context(), `SELECT count(*) FROM comments WHERE ticket_id=?`, ticketID).Scan(&total); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	rows, err := a.d.DB.Q().QueryContext(r.Context(), `SELECT id FROM comments WHERE ticket_id=? ORDER BY created_at `+direction+`,id `+direction+` LIMIT ? OFFSET ?`, ticketID, page.PerPage, page.Offset())
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			kferr.Write(w, kferr.Internal())
			return
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	items, err := present.Comments(r.Context(), a.d.DB.Q(), ids)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}
func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	var body struct {
		Body  string   `json:"body"`
		Files []string `json:"files"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	ticketID := r.PathValue("ticketId")
	var commentID string
	var events []model.WebhookEvent
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var callErr error
		commentID, events, callErr = a.d.Engine.AddComment(r.Context(), tx, engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID, body.Body, body.Files)
		if callErr != nil {
			if apiErr, ok := kferr.As(callErr); ok && apiErr.Code == "invalid_state" && apiErr.Message == "Comments are disabled for this workflow" {
				return kferr.ValidationFailed(map[string][]string{"base": {apiErr.Message}})
			}
			return callErr
		}
		if err := a.d.Events.Enqueue(r.Context(), tx, events); err != nil {
			return err
		}
		return pf23Audit(r, tx, p, "comment.create", commentID, ticketID, []string{"body", "files"}, a.d.Clock.Now())
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	item, err := present.Comment(r.Context(), a.d.DB.Q(), commentID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}
func (a *API) getComment(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	ticketID, commentID := r.PathValue("ticketId"), r.PathValue("commentId")
	if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	if _, err := store.EngLoadComment(r.Context(), a.d.DB.Q(), ticketID, commentID); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	item, err := present.Comment(r.Context(), a.d.DB.Q(), commentID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}
func (a *API) updateComment(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	ticketID, commentID := r.PathValue("ticketId"), r.PathValue("commentId")
	var events []model.WebhookEvent
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var callErr error
		events, callErr = a.d.Engine.EditComment(r.Context(), tx, engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID, commentID, body.Body)
		if callErr != nil {
			return callErr
		}
		if err := a.d.Events.Enqueue(r.Context(), tx, events); err != nil {
			return err
		}
		return pf23Audit(r, tx, p, "comment.update", commentID, ticketID, []string{"body"}, a.d.Clock.Now())
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	item, err := present.Comment(r.Context(), a.d.DB.Q(), commentID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}
func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	ticketID, commentID := r.PathValue("ticketId"), r.PathValue("commentId")
	var events []model.WebhookEvent
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var callErr error
		events, callErr = a.d.Engine.DeleteComment(r.Context(), tx, engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID, commentID)
		if callErr != nil {
			return callErr
		}
		if err := a.d.Events.Enqueue(r.Context(), tx, events); err != nil {
			return err
		}
		return pf23Audit(r, tx, p, "comment.delete", commentID, ticketID, []string{"id"}, a.d.Clock.Now())
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteEmpty(w)
}
