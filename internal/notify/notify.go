// Package notify produces punchflow's local notification inbox (PF-20).
//
// Notifications are written inside the same transaction as the ticket event
// that caused them: the Sink decorator wraps the webhook outbox sink, so every
// engine event that is enqueued for webhooks is also fanned out to the inbox
// without the engine or the HTTP handlers knowing about notifications. No
// email or chat delivery exists; the inbox is the only observable output.
package notify

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"punchflow/internal/engine"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Notification kinds stored in notifications.kind.
const (
	KindTicketApplied     = "ticket_applied"
	KindApprovalRequested = "approval_requested"
	KindTicketApproved    = "ticket_approved"
	KindTicketConfirmed   = "ticket_confirmed"
	KindTicketRejected    = "ticket_rejected"
	KindTicketDenied      = "ticket_denied"
	KindTicketCompleted   = "ticket_completed"
	KindTicketWithdrawn   = "ticket_withdrawn"
	KindTicketArchived    = "ticket_archived"
	KindTicketUpdated     = "ticket_updated"
	KindCommentCreated    = "comment_created"
	KindCommentMention    = "comment_mention"
)

// excerptRunes bounds the stored comment excerpt.
const excerptRunes = 140

// lifecycleKinds maps engine event types to the notification kind sent to the
// ticket's author and watchers.
var lifecycleKinds = map[string]string{
	model.WebhookEventTicketOpened:    KindTicketApplied,
	model.WebhookEventTicketApproved:  KindTicketApproved,
	model.WebhookEventTicketConfirmed: KindTicketConfirmed,
	model.WebhookEventTicketRejected:  KindTicketRejected,
	model.WebhookEventTicketDenied:    KindTicketDenied,
	model.WebhookEventTicketCompleted: KindTicketCompleted,
	model.WebhookEventTicketWithdrawn: KindTicketWithdrawn,
	model.WebhookEventTicketArchived:  KindTicketArchived,
	model.WebhookEventTicketUpdated:   KindTicketUpdated,
}

// requestEvents are the events after which a different set of approvers may be
// the current assignees, so approval requests are (re)evaluated.
var requestEvents = map[string]bool{
	model.WebhookEventTicketOpened:    true,
	model.WebhookEventTicketApproved:  true,
	model.WebhookEventTicketConfirmed: true,
	model.WebhookEventTicketRejected:  true,
}

var mentionPattern = regexp.MustCompile(`<@([0-9a-fA-F-]{36})>`)

// Viewer decides ticket visibility; *engine.Engine implements it.
type Viewer interface {
	CanView(ctx context.Context, q store.Querier, a engine.Actor, ticketID string) error
}

// Service fans engine events out to the notification inbox.
type Service struct {
	viewer Viewer
	clock  kftime.Clock
}

// New returns a Service. A nil clock selects the system clock.
func New(viewer Viewer, clock kftime.Clock) *Service {
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	return &Service{viewer: viewer, clock: clock}
}

// Fanout writes the notifications caused by events using q, which must be the
// transaction that produced the events. Recipients who can no longer see the
// ticket, inactive users and the acting user (except for approval requests)
// are skipped.
func (s *Service) Fanout(ctx context.Context, q store.Querier, events []model.WebhookEvent) error {
	run := &fanoutRun{service: s, q: q, eligible: map[string]map[string]bool{}}
	for _, event := range events {
		if event.TicketID == "" {
			continue
		}
		if err := run.event(ctx, event); err != nil {
			return fmt.Errorf("notify: fan out %s: %w", event.Type, err)
		}
	}
	return nil
}

// ApprovalRequestsForCurrentStep creates local approval-request notifications
// without creating a webhook event. Kickflow's raise operation requests the
// newly active approvers while intentionally sending no raising webhook.
func (s *Service) ApprovalRequestsForCurrentStep(ctx context.Context, q store.Querier, ticketID, actorID string) error {
	ticket, err := store.EngLoadTicket(ctx, q, ticketID)
	if err != nil {
		return err
	}
	run := &fanoutRun{service: s, q: q, eligible: map[string]map[string]bool{}}
	_, err = run.approvalRequests(ctx, ticket, model.WebhookEvent{Type: model.WebhookEventTicketUpdated, TicketID: ticketID, ActorUserID: actorID})
	return err
}

// fanoutRun caches per-call lookups so a batch of events does not repeat them.
type fanoutRun struct {
	service *Service
	q       store.Querier
	// eligible caches "may this user receive notifications for this ticket"
	// as ticket ID -> user ID -> answer.
	eligible map[string]map[string]bool
}

func (r *fanoutRun) event(ctx context.Context, event model.WebhookEvent) error {
	kind, lifecycle := lifecycleKinds[event.Type]
	isComment := event.Type == model.WebhookEventCommentCreated
	if !lifecycle && !isComment {
		return nil
	}
	ticket, err := store.EngLoadTicket(ctx, r.q, event.TicketID)
	if err != nil {
		// The ticket vanished within the transaction; nothing to notify about.
		return nil
	}
	audience, err := r.audience(ctx, ticket)
	if err != nil {
		return err
	}
	if lifecycle {
		// An approver who is asked to act on this event gets the request
		// instead of a second, generic notice for the same event.
		var asked map[string]bool
		if requestEvents[event.Type] {
			if asked, err = r.approvalRequests(ctx, ticket, event); err != nil {
				return err
			}
		}
		return r.send(ctx, ticket, kind, event, "", audience, asked)
	}
	return r.comment(ctx, ticket, event, audience)
}

// comment notifies mentioned users (comment_mention) and the remaining
// audience (comment_created).
func (r *fanoutRun) comment(ctx context.Context, ticket model.Ticket, event model.WebhookEvent, audience []string) error {
	if event.CommentID == "" {
		return nil
	}
	comment, err := store.EngLoadComment(ctx, r.q, ticket.ID, event.CommentID)
	if err != nil || comment.DeletedAt != nil {
		return nil
	}
	excerpt, err := r.excerpt(ctx, comment.Body)
	if err != nil {
		return err
	}
	mentioned := map[string]bool{}
	mentions := make([]string, 0, len(comment.MentionedUserIDs))
	for _, id := range comment.MentionedUserIDs {
		if !mentioned[id] {
			mentioned[id] = true
			mentions = append(mentions, id)
		}
	}
	if err = r.send(ctx, ticket, KindCommentMention, event, excerpt, mentions, nil); err != nil {
		return err
	}
	return r.send(ctx, ticket, KindCommentCreated, event, excerpt, audience, mentioned)
}

// approvalRequests notifies the unfinished assignees of the ticket's current
// actionable steps once per assignment (the assignee row and its last update
// form the idempotency key, so a send-back that re-activates a step notifies
// again while a partial approval does not repeat earlier requests). It returns
// the users who are asked, whether or not a new notification was written.
func (r *fanoutRun) approvalRequests(ctx context.Context, ticket model.Ticket, event model.WebhookEvent) (map[string]bool, error) {
	asked := map[string]bool{}
	if ticket.Status != model.TicketStatusInProgress {
		return asked, nil
	}
	steps, err := store.EngTicketSteps(ctx, r.q, ticket.ID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.StepOrder != ticket.CurrentStep || step.Status != model.StepStatusInProgress || step.ActionType == model.ActionTypeNone {
			continue
		}
		assignees, err := store.EngStepAssignees(ctx, r.q, step.ID)
		if err != nil {
			return nil, err
		}
		for _, assignee := range assignees {
			if assignee.Completed {
				continue
			}
			asked[assignee.UserID] = true
			key := fmt.Sprintf("approval:%s:%d", assignee.ID, store.Millis(assignee.UpdatedAt))
			if err = r.deliver(ctx, ticket, KindApprovalRequested, event, "", assignee.UserID, &key, true); err != nil {
				return nil, err
			}
		}
	}
	return asked, nil
}

// send delivers kind to every recipient not in skip, dropping the actor.
func (r *fanoutRun) send(ctx context.Context, ticket model.Ticket, kind string, event model.WebhookEvent, excerpt string, recipients []string, skip map[string]bool) error {
	seen := map[string]bool{}
	for _, userID := range recipients {
		if seen[userID] || skip[userID] {
			continue
		}
		seen[userID] = true
		if err := r.deliver(ctx, ticket, kind, event, excerpt, userID, nil, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *fanoutRun) deliver(ctx context.Context, ticket model.Ticket, kind string, event model.WebhookEvent, excerpt, userID string, dedupe *string, includeActor bool) error {
	if userID == event.ActorUserID && !includeActor {
		return nil
	}
	ok, err := r.canReceive(ctx, ticket, userID)
	if err != nil || !ok {
		return err
	}
	n := store.Notification{
		ID: store.NewID(), UserID: userID, Kind: kind, TicketID: ticket.ID,
		Excerpt: excerpt, DedupeKey: dedupe, CreatedAt: r.service.clock.Now(),
	}
	if event.CommentID != "" {
		comment := event.CommentID
		n.CommentID = &comment
	}
	if event.ActorUserID != "" {
		actor := event.ActorUserID
		n.ActorUserID = &actor
	}
	if _, err = store.InsertNotification(ctx, r.q, n); err != nil {
		return err
	}
	return nil
}

// canReceive reports whether the user is active and may read the ticket, so a
// notification never discloses a ticket the user cannot open.
func (r *fanoutRun) canReceive(ctx context.Context, ticket model.Ticket, userID string) (bool, error) {
	perTicket := r.eligible[ticket.ID]
	if perTicket == nil {
		perTicket = map[string]bool{}
		r.eligible[ticket.ID] = perTicket
	}
	if answer, ok := perTicket[userID]; ok {
		return answer, nil
	}
	answer, err := r.checkEligible(ctx, ticket, userID)
	if err != nil {
		return false, err
	}
	perTicket[userID] = answer
	return answer, nil
}

func (r *fanoutRun) checkEligible(ctx context.Context, ticket model.Ticket, userID string) (bool, error) {
	user, err := store.GetUser(ctx, r.q, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if user.Status != model.UserStatusActive {
		return false, nil
	}
	actor, err := engine.ActorForUser(ctx, r.q, userID)
	if err != nil {
		return false, err
	}
	err = r.service.viewer.CanView(ctx, r.q, actor, ticket.ID)
	if err == nil {
		return true, nil
	}
	if apiErr, ok := kferr.As(err); ok && (apiErr.Code == "invisible_ticket" || apiErr.Code == "resource_not_found") {
		return false, nil
	}
	return false, err
}

// excerpt renders a comment body for the inbox: mention markup is replaced by
// the mentioned user's name and the text is truncated.
func (r *fanoutRun) excerpt(ctx context.Context, body string) (string, error) {
	var lookupErr error
	text := mentionPattern.ReplaceAllStringFunc(body, func(match string) string {
		id := mentionPattern.FindStringSubmatch(match)[1]
		user, err := store.GetUser(ctx, r.q, id)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				lookupErr = err
			}
			return "@user"
		}
		return "@" + strings.TrimSpace(user.LastName+" "+user.FirstName)
	})
	if lookupErr != nil {
		return "", lookupErr
	}
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) > excerptRunes {
		text = string([]rune(text)[:excerptRunes]) + "..."
	}
	return text, nil
}
