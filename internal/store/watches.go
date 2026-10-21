package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Ticket watch states persisted in ticket_watches.state.
const (
	WatchStateWatching  = "watching"
	WatchStateUnwatched = "unwatched"
)

// TicketWatchState returns the explicit watch choice of the user, or "" when
// the user never chose (the default then applies).
func TicketWatchState(ctx context.Context, q Querier, ticketID, userID string) (string, error) {
	var state string
	err := q.QueryRowContext(ctx, `SELECT state FROM ticket_watches WHERE ticket_id=? AND user_id=?`, ticketID, userID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: read ticket watch: %w", err)
	}
	return state, nil
}

// SetTicketWatchState upserts the explicit watch choice of the user.
func SetTicketWatchState(ctx context.Context, q Querier, ticketID, userID, state string, now time.Time) error {
	if state != WatchStateWatching && state != WatchStateUnwatched {
		return fmt.Errorf("store: invalid watch state %q", state)
	}
	ms := Millis(now)
	_, err := q.ExecContext(ctx, `INSERT INTO ticket_watches(ticket_id,user_id,state,created_at,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(ticket_id,user_id) DO UPDATE SET state=excluded.state, updated_at=excluded.updated_at`, ticketID, userID, state, ms, ms)
	if err != nil {
		return fmt.Errorf("store: write ticket watch: %w", err)
	}
	return nil
}

// SetTicketWatchStateIfUnset applies an automatic default without replacing a
// user's explicit watch choice.
func SetTicketWatchStateIfUnset(ctx context.Context, q Querier, ticketID, userID, state string, now time.Time) error {
	if state != WatchStateWatching && state != WatchStateUnwatched {
		return fmt.Errorf("store: invalid watch state %q", state)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO ticket_watches(ticket_id,user_id,state,created_at,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(ticket_id,user_id) DO NOTHING`, ticketID, userID, state, Millis(now), Millis(now))
	if err != nil {
		return fmt.Errorf("store: apply default ticket watch: %w", err)
	}
	return nil
}

// TicketWatchStates returns every explicit watch row of the ticket keyed by
// user ID.
func TicketWatchStates(ctx context.Context, q Querier, ticketID string) (map[string]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id,state FROM ticket_watches WHERE ticket_id=?`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: list ticket watches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var userID, state string
		if err = rows.Scan(&userID, &state); err != nil {
			return nil, fmt.Errorf("store: scan ticket watch: %w", err)
		}
		out[userID] = state
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ticket watches: %w", err)
	}
	return out, nil
}

// TicketApproverUserIDs returns the distinct users who are assignees of any
// step of the ticket, followed by the users who acted on their behalf, in
// assignment order. They are the ticket's approvers for automatic watching.
func TicketApproverUserIDs(ctx context.Context, q Querier, ticketID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id,actual_approver_id FROM ticket_assignees WHERE ticket_id=? ORDER BY created_at,id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: list ticket approvers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for rows.Next() {
		var userID string
		var actual sql.NullString
		if err = rows.Scan(&userID, &actual); err != nil {
			return nil, fmt.Errorf("store: scan ticket approver: %w", err)
		}
		add(userID)
		add(actual.String)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ticket approvers: %w", err)
	}
	return out, nil
}
