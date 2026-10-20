package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestCreationRestricted(t *testing.T) {
	e := pf14Setup(t)
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "purchase-request")
	if err != nil {
		t.Fatal(err)
	}
	// Creation restriction is evaluated before form validation. Use a draft with
	// the finance manager's own team so this assertion covers the permission.
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, createErr := e.engine.Create(e.ctx, tx, e.actor("finance-manager"), CreateInput{
			Status:       model.TicketStatusDraft,
			WorkflowID:   workflow.ID,
			AuthorTeamID: e.team("finance"),
		})
		return createErr
	})
	assertPF14ErrorCode(t, err, "ticket_create_restricted")
}

func TestAdminWorking(t *testing.T) {
	e := pf14Setup(t)
	id := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE tenant SET mode=?`, model.TenantModeAdminWorking)
		return err
	})
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return err
	})
	assertPF14ErrorCode(t, err, "admin_is_working")
}

func TestVisibility(t *testing.T) {
	e := pf14Setup(t)
	for _, submit := range []bool{true, false} {
		status := model.TicketStatusInProgress
		if !submit {
			status = model.TicketStatusDraft
		}
		ticketID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", status, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
		if err := e.engine.CanView(e.ctx, e.db.Q(), e.actor("sales-rep"), ticketID); err != nil {
			t.Fatalf("author cannot view submit=%t ticket: %v", submit, err)
		}
		reader := Actor{UserID: e.user("hr-manager"), Permissions: map[string]bool{model.PermissionTicketRead: true}}
		if err := e.engine.CanView(e.ctx, e.db.Q(), reader, ticketID); err != nil {
			t.Fatalf("ticket_read actor cannot view submit=%t ticket: %v", submit, err)
		}
		err := e.engine.CanView(e.ctx, e.db.Q(), e.actor("engineer"), ticketID)
		assertPF14ErrorCode(t, err, "invisible_ticket")
	}
}

func TestArchive(t *testing.T) {
	e := pf14Setup(t)
	draftID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusDraft, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
	var events []model.WebhookEvent
	e.apply(func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Archive(e.ctx, tx, e.actor("sales-rep"), draftID)
		return err
	})
	if got := e.ticket(draftID).Status; got != model.TicketStatusArchived {
		t.Fatalf("draft archive status=%s, want %s", got, model.TicketStatusArchived)
	}
	if len(events) != 1 || events[0].Type != model.WebhookEventTicketArchived {
		t.Fatalf("draft archive events=%+v", events)
	}

	inProgressID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Archive(e.ctx, tx, e.actor("sales-rep"), inProgressID)
		return err
	})
	assertPF14ErrorCode(t, err, "missing_permission")

	completedID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
	for _, code := range []string{"sales-manager", "finance-manager"} {
		e.apply(func(tx *store.Tx) error {
			_, err := e.engine.Approve(e.ctx, tx, e.actor(code), completedID, nil)
			return err
		})
	}
	writer := Actor{UserID: e.user("hr-manager"), Permissions: map[string]bool{model.PermissionTicketRead: true, model.PermissionTicketWrite: true}}
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Archive(e.ctx, tx, writer, completedID)
		return err
	})
	if got := e.ticket(completedID).Status; got != model.TicketStatusArchived {
		t.Fatalf("completed archive status=%s, want %s", got, model.TicketStatusArchived)
	}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Archive(e.ctx, tx, writer, completedID)
		return err
	})
	assertPF14ErrorCode(t, err, "invalid_state")
}

func assertPF14ErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	api, ok := kferr.As(err)
	if !ok || api.Code != want {
		t.Fatalf("error=%v, want API error code %q", err, want)
	}
}
