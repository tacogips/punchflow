package notify

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Watch sources reported by WatchStatus.
const (
	// SourceManual means the user explicitly watched the ticket.
	SourceManual = "manual"
	// SourceWorkflow means the user watches through the workflow definition.
	SourceWorkflow = "workflow"
	// SourceNone is reported when the user does not watch the ticket.
	SourceNone = ""
)

// WatchStatus describes whether a user watches a ticket and why.
type WatchStatus struct {
	Watching bool
	Source   string
	// WorkflowWatcher is true when the workflow definition lists the user,
	// independent of an explicit opt-out.
	WorkflowWatcher bool
}

// WorkflowWatcherIDs resolves the workflow definition's watchers (users, and
// the active members of watcher teams) to user IDs, in definition order.
func WorkflowWatcherIDs(ctx context.Context, q store.Querier, workflowID string) ([]string, error) {
	def, err := store.WorkflowDefByID(ctx, q, workflowID)
	if err != nil {
		return nil, fmt.Errorf("notify: load workflow: %w", err)
	}
	var out []string
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, watcher := range def.Watchers {
		if watcher.UserCode != "" {
			id, err := store.UserIDByCode(ctx, q, watcher.UserCode)
			if err == nil {
				add(id)
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("notify: resolve watcher user: %w", err)
			}
		}
		if watcher.TeamCode != "" {
			chart, err := store.ActiveOrgChartID(ctx, q)
			if err != nil {
				return nil, fmt.Errorf("notify: active org chart: %w", err)
			}
			teamID, err := store.TeamIDByCode(ctx, q, chart, watcher.TeamCode)
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("notify: resolve watcher team: %w", err)
			}
			members, err := store.EngUsersByTeam(ctx, q, teamID, false)
			if err != nil {
				return nil, fmt.Errorf("notify: watcher team members: %w", err)
			}
			for _, id := range members {
				add(id)
			}
		}
	}
	return out, nil
}

// EffectiveWatchers returns the user IDs watching the ticket: workflow-level
// watchers that did not opt out, followed by explicit subscribers.
func EffectiveWatchers(ctx context.Context, q store.Querier, ticket model.Ticket) ([]string, error) {
	workflow, err := WorkflowWatcherIDs(ctx, q, ticket.WorkflowID)
	if err != nil {
		return nil, err
	}
	states, err := store.TicketWatchStates(ctx, q, ticket.ID)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range workflow {
		if states[id] == store.WatchStateUnwatched || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	manual := make([]string, 0, len(states))
	for id, state := range states {
		if state == store.WatchStateWatching && !seen[id] {
			manual = append(manual, id)
		}
	}
	sort.Strings(manual)
	return append(out, manual...), nil
}

// WatchStatusFor reports whether the user watches the ticket.
func WatchStatusFor(ctx context.Context, q store.Querier, ticket model.Ticket, userID string) (WatchStatus, error) {
	workflow, err := WorkflowWatcherIDs(ctx, q, ticket.WorkflowID)
	if err != nil {
		return WatchStatus{}, err
	}
	status := WatchStatus{}
	for _, id := range workflow {
		if id == userID {
			status.WorkflowWatcher = true
		}
	}
	state, err := store.TicketWatchState(ctx, q, ticket.ID, userID)
	if err != nil {
		return WatchStatus{}, err
	}
	switch {
	case state == store.WatchStateWatching:
		status.Watching, status.Source = true, SourceManual
		if status.WorkflowWatcher {
			status.Source = SourceWorkflow
		}
	case state == "" && status.WorkflowWatcher:
		status.Watching, status.Source = true, SourceWorkflow
	}
	return status, nil
}

// audience returns the users informed about ticket activity: the author, the
// proxy client and the effective watchers, without duplicates.
func (r *fanoutRun) audience(ctx context.Context, ticket model.Ticket) ([]string, error) {
	var out []string
	if ticket.AuthorID != nil {
		out = append(out, *ticket.AuthorID)
	}
	if ticket.ProxyClientUserID != nil {
		out = append(out, *ticket.ProxyClientUserID)
	}
	watchers, err := EffectiveWatchers(ctx, r.q, ticket)
	if err != nil {
		return nil, err
	}
	return append(out, watchers...), nil
}
