package uiapi

import (
	"context"
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/notify"
	"punchflow/internal/store"
)

type watchJSON struct {
	TicketID        string `json:"ticketId"`
	Watching        bool   `json:"watching"`
	Source          string `json:"source"`
	WorkflowWatcher bool   `json:"workflowWatcher"`
	OptedOut        bool   `json:"optedOut"`
}

func (a *API) getWatch(w http.ResponseWriter, r *http.Request) {
	a.watch(w, r, nil)
}

func (a *API) watchTicket(w http.ResponseWriter, r *http.Request) {
	state := store.WatchStateWatching
	a.watch(w, r, &state)
}

func (a *API) unwatchTicket(w http.ResponseWriter, r *http.Request) {
	state := store.WatchStateUnwatched
	a.watch(w, r, &state)
}

// watch reads, or when state is non-nil first sets, the caller's watch choice.
func (a *API) watch(w http.ResponseWriter, r *http.Request, state *string) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	ticketID := r.PathValue("ticketId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	var out watchJSON
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := a.d.Engine.CanView(r.Context(), tx, actor, ticketID); err != nil {
			return err
		}
		if state != nil {
			if err := store.SetTicketWatchState(r.Context(), tx, ticketID, p.User.ID, *state, a.d.Clock.Now()); err != nil {
				return err
			}
		}
		var err error
		out, err = a.watchStatus(r.Context(), tx, ticketID, p.User.ID)
		return err
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) watchStatus(ctx context.Context, q store.Querier, ticketID, userID string) (watchJSON, error) {
	ticket, err := store.EngLoadTicket(ctx, q, ticketID)
	if err != nil {
		return watchJSON{}, kferr.ResourceNotFound()
	}
	status, err := notify.WatchStatusFor(ctx, q, ticket, userID)
	if err != nil {
		return watchJSON{}, err
	}
	return watchJSON{TicketID: ticketID, Watching: status.Watching, Source: status.Source, WorkflowWatcher: status.WorkflowWatcher, OptedOut: status.OptedOut}, nil
}
