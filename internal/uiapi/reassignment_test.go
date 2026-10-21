package uiapi_test

import (
	"net/http"
	"testing"
	"time"

	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/notify"
	"punchflow/internal/testutil"
)

func TestApprovalRequestRepeatsOnlyAfterReassignment(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Loop", purpose: "send back"})
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "approval_requested", id) != 1 {
		t.Fatalf("manager inbox = %v", kinds(got))
	}

	// Replaying the same event does not duplicate the request.
	service := notify.New(engine.New(e.Clock), e.Clock)
	replay := []model.WebhookEvent{{Type: model.WebhookEventTicketOpened, TicketID: id, ActorUserID: e.UserID(t, "sales-rep")}}
	if err := service.Fanout(t.Context(), e.DB.Q(), replay); err != nil {
		t.Fatal(err)
	}
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "approval_requested", id) != 1 {
		t.Fatalf("replayed event duplicated the request: %v", kinds(got))
	}

	// Sending the ticket back to the first approver asks them again.
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	e.Clock.Advance(time.Minute)
	if response := e.post(t, testutil.Finance, "/v1/tickets/"+id+"/reject", map[string]any{"to": 1}); response.Status != http.StatusOK {
		t.Fatalf("send back: %d %s", response.Status, response.Body)
	}
	manager := e.inbox(t, testutil.Manager, "")
	if countKind(manager, "approval_requested", id) != 2 {
		t.Fatalf("manager inbox after send back = %v, want a second approval_requested", kinds(manager))
	}
	if countKind(e.inbox(t, testutil.Employee, ""), "ticket_rejected", id) != 1 {
		t.Fatalf("author was not told about the send back")
	}
}
