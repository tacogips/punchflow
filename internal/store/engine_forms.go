package store

import (
	"context"
	"database/sql"
	"fmt"
)

// EngIsTicketAssignee reports whether the user is, or was, an assignee of any
// step of the ticket.
func EngIsTicketAssignee(ctx context.Context, q Querier, ticketID, userID string) (bool, error) {
	var found int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM ticket_assignees WHERE ticket_id=? AND user_id=? LIMIT 1`, ticketID, userID).Scan(&found)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("store: lookup ticket assignee: %w", err)
	}
	return true, nil
}

// InputAttachment associates uploaded files (by signed ID) with one ticket
// input location: a form field, or one detail row.
type InputAttachment struct {
	FormFieldID string
	SlipItemID  string
	SignedIDs   []string
}

// EngSyncInputAttachments replaces every form-field and detail-row file
// association of the ticket (comment attachments are untouched) with the
// supplied set, so replacing or removing a file input also removes its
// association. Unknown signed IDs are skipped; callers validate them first.
func EngSyncInputAttachments(ctx context.Context, q Querier, ticketID string, items []InputAttachment, now int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM file_attachments WHERE ticket_id=? AND comment_id IS NULL AND (form_field_id IS NOT NULL OR slip_item_id IS NOT NULL)`, ticketID); err != nil {
		return fmt.Errorf("store: clear input attachments: %w", err)
	}
	for _, item := range items {
		seen := map[string]bool{}
		for _, signed := range item.SignedIDs {
			if seen[signed] {
				continue
			}
			seen[signed] = true
			var fileID string
			if err := q.QueryRowContext(ctx, `SELECT id FROM files WHERE signed_id=?`, signed).Scan(&fileID); err != nil {
				continue
			}
			var field, slip any
			if item.FormFieldID != "" {
				field = item.FormFieldID
			}
			if item.SlipItemID != "" {
				slip = item.SlipItemID
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO file_attachments(file_id,ticket_id,comment_id,form_field_id,slip_item_id,created_at) VALUES(?,?,NULL,?,?,?)`, fileID, ticketID, field, slip, now); err != nil {
				return fmt.Errorf("store: attach input file: %w", err)
			}
		}
	}
	return nil
}

// EngUnlinkTickets removes the reciprocal related-ticket links between a and b.
func EngUnlinkTickets(ctx context.Context, q Querier, a, b string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM ticket_links WHERE (ticket_id=? AND linked_ticket_id=?) OR (ticket_id=? AND linked_ticket_id=?)`, a, b, b, a); err != nil {
		return fmt.Errorf("store: unlink engine tickets: %w", err)
	}
	return nil
}
