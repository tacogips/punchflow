package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// NF-15: a workflow with commentingEnabled=false rejects new comments with
// validation_failed and an errors.base entry (kickflow changelog 2026-06-22).
func TestAddCommentDisabledIsValidationFailed(t *testing.T) {
	e := pf14Setup(t)
	ticketID := pf14CreateWorkflowTicket(t, e, "leave-request", "sales-rep", model.TicketStatusInProgress, map[string]any{
		"start-date": "2025-03-03",
		"end-date":   "2025-03-04",
		"reason":     "vacation",
	}, nil)
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, addErr := e.engine.AddComment(e.ctx, tx, e.actor("sales-rep"), ticketID, "hello", nil)
		return addErr
	})
	assertPF14FieldError(t, err, "validation_failed", "base")
}
