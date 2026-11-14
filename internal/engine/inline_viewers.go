package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// InlineViewer is one entry of the `viewers` array accepted by createTicket and
// updateTicket (kickflow API changelog, 2026-08-05: viewers[].userId).
type InlineViewer struct {
	UserID string `json:"userId"`
}

// applyInlineViewers persists the `viewers` and `watcherUserId` request fields
// of createTicket/updateTicket after the ticket row exists.
//
//   - viewers is presence-aware: nil leaves the ticket's viewers untouched,
//     otherwise the set of explicit user viewers becomes exactly the listed
//     users (an empty array clears them). Team viewers are never changed.
//   - watcherUserId subscribes that user to the ticket (an explicit "watching"
//     row, overriding an earlier opt-out). Absent and null are both a no-op.
//
// Users must exist and not be deleted (deactivated); suspended users are
// accepted, as the changelog states. Violations are 422 validation_failed.
// The caller runs inside the request transaction, so a failure rolls back the
// whole create/update.
func applyInlineViewers(ctx context.Context, tx *store.Tx, a Actor, def model.WorkflowDef, ticketID string, viewers *[]InlineViewer, watcherUserID *string, now time.Time) error {
	if viewers == nil && watcherUserID == nil {
		return nil
	}
	var viewerIDs []string
	if viewers != nil {
		// Same rule as POST/DELETE .../viewers; an empty list that changes
		// nothing is not an edit.
		existing, err := store.TicketViewers(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		edits := len(*viewers) > 0
		for _, viewer := range existing {
			edits = edits || viewer.UserID != nil
		}
		if edits && !def.AllowEditingOfViewers && !a.Permissions[model.PermissionTicketWrite] {
			return kferr.MissingPermission("Missing permission")
		}
		seen := map[string]bool{}
		for i, viewer := range *viewers {
			key := fmt.Sprintf("viewers[%d].userId", i)
			if err := requireExistingUser(ctx, tx, key, viewer.UserID); err != nil {
				return err
			}
			if !seen[viewer.UserID] {
				seen[viewer.UserID] = true
				viewerIDs = append(viewerIDs, viewer.UserID)
			}
		}
	}
	if watcherUserID != nil {
		if err := requireExistingUser(ctx, tx, "watcherUserId", *watcherUserID); err != nil {
			return err
		}
	}
	if viewers != nil {
		if err := replaceUserViewers(ctx, tx, ticketID, viewerIDs); err != nil {
			return err
		}
	}
	if watcherUserID != nil {
		if err := store.SetTicketWatchState(ctx, tx, ticketID, *watcherUserID, store.WatchStateWatching, now); err != nil {
			return err
		}
		return addUserViewer(ctx, tx, ticketID, *watcherUserID)
	}
	return nil
}

// requireExistingUser rejects blank, unknown and deleted (deactivated) users.
func requireExistingUser(ctx context.Context, q store.Querier, field, userID string) error {
	if userID == "" {
		return kferr.ValidationFailed(map[string][]string{field: {"is required"}})
	}
	user, err := store.GetUser(ctx, q, userID)
	if errors.Is(err, store.ErrNotFound) {
		return kferr.ValidationFailed(map[string][]string{field: {"does not exist"}})
	}
	if err != nil {
		return fmt.Errorf("engine: load viewer user: %w", err)
	}
	if user.Status == model.UserStatusDeactivated || user.DeactivatedAt != nil {
		return kferr.ValidationFailed(map[string][]string{field: {"does not exist"}})
	}
	return nil
}

// replaceUserViewers makes userIDs the exact set of user viewers of the ticket.
// Rows of users that stay keep their identity; duplicates are collapsed.
func replaceUserViewers(ctx context.Context, tx *store.Tx, ticketID string, userIDs []string) error {
	existing, err := store.TicketViewers(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, id := range userIDs {
		want[id] = true
	}
	kept := map[string]bool{}
	for _, viewer := range existing {
		if viewer.UserID == nil {
			continue
		}
		if !want[*viewer.UserID] || kept[*viewer.UserID] {
			if err = store.TicketQueryDeleteViewer(ctx, tx, ticketID, viewer.ID); err != nil {
				return err
			}
			continue
		}
		kept[*viewer.UserID] = true
	}
	for _, id := range userIDs {
		if kept[id] {
			continue
		}
		userID := id
		if err = store.TicketQueryInsertViewer(ctx, tx, model.TicketViewer{ID: store.NewID(), TicketID: ticketID, UserID: &userID}); err != nil {
			return err
		}
	}
	return nil
}

func addUserViewer(ctx context.Context, tx *store.Tx, ticketID, userID string) error {
	viewers, err := store.TicketViewers(ctx, tx, ticketID)
	if err != nil {
		return err
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == userID {
			return nil
		}
	}
	return store.TicketQueryInsertViewer(ctx, tx, model.TicketViewer{ID: store.NewID(), TicketID: ticketID, UserID: &userID})
}
