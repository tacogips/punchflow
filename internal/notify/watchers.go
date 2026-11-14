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
	// SourceApplicant means the user is the ticket's author or proxy client,
	// who watch the ticket automatically.
	SourceApplicant = "applicant"
	// SourceApprover means the user is, or was, an assignee of an approval step
	// of the ticket, and so watches it automatically.
	SourceApprover = "approver"
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
	// OptedOut is true when the user would watch automatically (applicant,
	// approver or workflow watcher) but explicitly stopped watching.
	OptedOut bool
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

// subscriptions is the single effective-subscription calculation shared by
// the watch status and by notification delivery.
//
// Automatic subscribers, in order, are the applicant (author, proxy client),
// the approvers (assignees and the users who acted for them) and the workflow
// definition's watchers; an explicit "unwatched" row removes any of them.
// Explicit "watching" rows add the remaining subscribers.
type subscriptions struct {
	order []string
	// source maps every subscribed user to the reason that is reported.
	source map[string]string
	// automatic maps users who watch by default to their default source,
	// whether or not they opted out.
	automatic map[string]string
	// workflow marks the definition's watchers regardless of opt-out.
	workflow map[string]bool
	states   map[string]string
}

func resolveSubscriptions(ctx context.Context, q store.Querier, ticket model.Ticket) (subscriptions, error) {
	subs := subscriptions{source: map[string]string{}, automatic: map[string]string{}, workflow: map[string]bool{}}
	var candidates []string
	addAutomatic := func(id, source string) {
		if _, ok := subs.automatic[id]; !ok {
			subs.automatic[id] = source
			candidates = append(candidates, id)
		}
	}
	if ticket.AuthorID != nil {
		addAutomatic(*ticket.AuthorID, SourceApplicant)
	}
	if ticket.ProxyClientUserID != nil {
		addAutomatic(*ticket.ProxyClientUserID, SourceApplicant)
	}
	approvers, err := store.TicketApproverUserIDs(ctx, q, ticket.ID)
	if err != nil {
		return subs, err
	}
	for _, id := range approvers {
		addAutomatic(id, SourceApprover)
	}
	workflow, err := WorkflowWatcherIDs(ctx, q, ticket.WorkflowID)
	if err != nil {
		return subs, err
	}
	for _, id := range workflow {
		subs.workflow[id] = true
		addAutomatic(id, SourceWorkflow)
	}
	subs.states, err = store.TicketWatchStates(ctx, q, ticket.ID)
	if err != nil {
		return subs, err
	}
	for _, id := range candidates {
		if subs.states[id] == store.WatchStateUnwatched {
			continue
		}
		subs.order = append(subs.order, id)
		subs.source[id] = subs.automatic[id]
	}
	manual := make([]string, 0, len(subs.states))
	for id, state := range subs.states {
		if _, auto := subs.automatic[id]; state == store.WatchStateWatching && !auto {
			manual = append(manual, id)
		}
	}
	sort.Strings(manual)
	for _, id := range manual {
		subs.order = append(subs.order, id)
		subs.source[id] = SourceManual
	}
	return subs, nil
}

// EffectiveWatchers returns the user IDs subscribed to the ticket: applicant,
// approvers and workflow watchers who did not opt out, followed by explicit
// subscribers.
func EffectiveWatchers(ctx context.Context, q store.Querier, ticket model.Ticket) ([]string, error) {
	subs, err := resolveSubscriptions(ctx, q, ticket)
	if err != nil {
		return nil, err
	}
	return subs.order, nil
}

// WatchStatusFor reports whether the user watches the ticket, using the same
// calculation as EffectiveWatchers.
func WatchStatusFor(ctx context.Context, q store.Querier, ticket model.Ticket, userID string) (WatchStatus, error) {
	subs, err := resolveSubscriptions(ctx, q, ticket)
	if err != nil {
		return WatchStatus{}, err
	}
	source, watching := subs.source[userID]
	_, automatic := subs.automatic[userID]
	return WatchStatus{
		Watching:        watching,
		Source:          source,
		WorkflowWatcher: subs.workflow[userID],
		OptedOut:        automatic && !watching,
	}, nil
}

// audience returns the users informed about ticket activity: the effective
// watchers, which already include the applicant, proxy client and approvers
// unless they opted out.
func (r *fanoutRun) audience(ctx context.Context, ticket model.Ticket) ([]string, error) {
	return EffectiveWatchers(ctx, r.q, ticket)
}
