package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestR5SubmissionCommentStoredAndEmittedAfterLifecycleEvents(t *testing.T) {
	e := pf14Setup(t)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
	comment := "Submitted after review"
	var events []model.WebhookEvent
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Submit(e.ctx, tx, e.actor("sales-rep"), id, &comment)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "submit with comment", events, model.WebhookEventTicketOpened, model.WebhookEventCommentCreated)
	var body string
	if err = e.db.Q().QueryRowContext(e.ctx, `SELECT body FROM comments WHERE ticket_id=?`, id).Scan(&body); err != nil || body != comment {
		t.Fatalf("stored comment=%q err=%v", body, err)
	}
}

func TestR5CancellationCommentStoredAndEmittedAfterTicketUpdate(t *testing.T) {
	e := pf14Setup(t)
	pf15ThreeStepRoute(t, e)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "sales-manager", id)
	comment := "Please review my approval"
	var events []model.WebhookEvent
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = e.engine.CancelApproval(e.ctx, tx, e.actor("sales-manager"), id, &comment)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "cancel approval with comment", events, model.WebhookEventTicketUpdated, model.WebhookEventCommentCreated)
	var body string
	if err = e.db.Q().QueryRowContext(e.ctx, `SELECT body FROM comments WHERE ticket_id=?`, id).Scan(&body); err != nil || body != comment {
		t.Fatalf("stored comment=%q err=%v", body, err)
	}
}

func TestR5SubmissionCommentHonorsCommentingEnabled(t *testing.T) {
	e := pf14Setup(t)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
	compatSetWorkflow(t, e, "expense-claim", "$.commentingEnabled", false)
	comment := "not allowed"
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Submit(e.ctx, tx, e.actor("sales-rep"), id, &comment)
		return err
	})
	assertPF14ErrorCode(t, err, "validation_failed")
	if got := e.ticket(id).Status; got != model.TicketStatusDraft {
		t.Fatalf("failed comment submission persisted status %q", got)
	}
}
