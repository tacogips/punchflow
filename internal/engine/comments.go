package engine

import (
	"context"
	"regexp"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

var pf14MentionPattern = regexp.MustCompile(`<@([0-9a-fA-F-]{36})>`)

func (e *Engine) pf14AddComment(ctx context.Context, tx *store.Tx, a Actor, id, body string, files []string) (string, []model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return "", nil, err
	}
	if len(files) > 10 {
		return "", nil, kferr.ValidationFailed(map[string][]string{"files": {"must contain at most 10 files"}})
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return "", nil, err
	}
	if !def.CommentingEnabled {
		// kickflow (changelog 2026-06-22): 422 validation_failed with errors.base.
		return "", nil, kferr.ValidationFailed(map[string][]string{"base": {"Comments are disabled for this workflow"}})
	}
	if strings.TrimSpace(body) == "" && len(files) == 0 {
		return "", nil, kferr.ValidationFailed(map[string][]string{"body": {"is required"}})
	}
	mentioned := []string{}
	for _, m := range pf14MentionPattern.FindAllStringSubmatch(body, -1) {
		if !pf14Contains(mentioned, m[1]) {
			if _, err = store.GetUser(ctx, tx, m[1]); err != nil {
				return "", nil, kferr.ValidationFailed(map[string][]string{"body": {"contains an unknown mentioned user"}})
			}
			mentioned = append(mentioned, m[1])
		}
	}
	now := e.clock.Now()
	idComment := store.NewID()
	comment := model.Comment{ID: idComment, TicketID: id, UserID: a.UserID, Body: body, MentionedUserIDs: mentioned, CreatedAt: now, UpdatedAt: now}
	if err = store.EngAddComment(ctx, tx, comment); err != nil {
		return "", nil, err
	}
	if err = store.EngAttachCommentFiles(ctx, tx, id, idComment, files, store.Millis(now)); err != nil {
		return "", nil, err
	}
	event := pf14Event(model.WebhookEventCommentCreated, id, a.UserID)
	event.CommentID = idComment
	return idComment, []model.WebhookEvent{event}, nil
}
func (e *Engine) pf14EditComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID, body string) ([]model.WebhookEvent, error) {
	if _, err := e.pf14Guard(ctx, tx, a, ticketID); err != nil {
		return nil, err
	}
	comment, err := store.EngLoadComment(ctx, tx, ticketID, commentID)
	if err != nil {
		return nil, kferr.ResourceNotFound()
	}
	if comment.UserID != a.UserID {
		return nil, kferr.MissingPermission("Only the comment author may edit this comment")
	}
	if comment.DeletedAt != nil {
		return nil, kferr.InvalidState("Deleted comments cannot be edited")
	}
	if strings.TrimSpace(body) == "" {
		return nil, kferr.ValidationFailed(map[string][]string{"body": {"is required"}})
	}
	comment.Body = body
	comment.MentionedUserIDs = pf14Mentions(body)
	comment.UpdatedAt = e.clock.Now()
	if err = store.EngUpdateComment(ctx, tx, comment); err != nil {
		return nil, err
	}
	event := pf14Event(model.WebhookEventCommentUpdated, ticketID, a.UserID)
	event.CommentID = commentID
	return []model.WebhookEvent{event}, nil
}
func (e *Engine) pf14DeleteComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID string) ([]model.WebhookEvent, error) {
	if _, err := e.pf14Guard(ctx, tx, a, ticketID); err != nil {
		return nil, err
	}
	comment, err := store.EngLoadComment(ctx, tx, ticketID, commentID)
	if err != nil {
		return nil, kferr.ResourceNotFound()
	}
	if comment.UserID != a.UserID {
		return nil, kferr.MissingPermission("Only the comment author may delete this comment")
	}
	if comment.DeletedAt != nil {
		return nil, kferr.InvalidState("Comment is already deleted")
	}
	now := e.clock.Now()
	comment.Body = ""
	comment.MentionedUserIDs = []string{}
	comment.DeletedAt = &now
	comment.UpdatedAt = now
	if err = store.EngUpdateComment(ctx, tx, comment); err != nil {
		return nil, err
	}
	event := pf14Event(model.WebhookEventCommentUpdated, ticketID, a.UserID)
	event.CommentID = commentID
	return []model.WebhookEvent{event}, nil
}
func pf14Mentions(body string) []string {
	out := []string{}
	for _, m := range pf14MentionPattern.FindAllStringSubmatch(body, -1) {
		if !pf14Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}
