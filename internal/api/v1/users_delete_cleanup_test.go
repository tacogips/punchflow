package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func ticketStatus(t *testing.T, env *testutil.Env, id string) string {
	t.Helper()
	return decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets/"+id, nil))["status"].(string)
}

func outstandingAssignments(t *testing.T, env *testutil.Env, userCode string) int {
	t.Helper()
	var n int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM ticket_assignees WHERE user_id=? AND completed=0`, env.UserID(t, userCode)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteUserArchivesOpenTicketsAndEmitsWebhooks(t *testing.T) {
	env := testutil.New(t)
	env.RegisterReceiverWebhook(t, "s3cret", "ticket_archived")
	draft := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	inProgress := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	rejected := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejected+"/reject", map[string]any{"to": 0}); res.Status != http.StatusOK {
		t.Fatalf("reject: %d %s", res.Status, res.Body)
	}
	denied := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+denied+"/deny", nil); res.Status != http.StatusOK {
		t.Fatalf("deny: %d %s", res.Status, res.Body)
	}
	others := pf22CreateTicket(t, env, testutil.Finance, "in_progress", 5000)

	if got := ticketStatus(t, env, rejected); got != "rejected" {
		t.Fatalf("precondition: rejected ticket status = %s", got)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+env.UserID(t, "sales-rep"), nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete user: %d %s", deleted.Status, deleted.Body)
	}
	for name, id := range map[string]string{"draft": draft, "in_progress": inProgress, "rejected": rejected} {
		if got := ticketStatus(t, env, id); got != "archived" {
			t.Errorf("%s ticket status after author deletion = %s, want archived", name, got)
		}
	}
	if got := ticketStatus(t, env, denied); got != "denied" {
		t.Errorf("denied ticket must keep its terminal status, got %s", got)
	}
	if got := ticketStatus(t, env, others); got != "in_progress" {
		t.Errorf("another author's ticket changed to %s", got)
	}

	env.DeliverWebhooks(t)
	archivedEvents := map[string]bool{}
	for _, request := range env.Receiver.Requests() {
		var payload struct {
			EventType string `json:"eventType"`
			Data      struct {
				Ticket struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"ticket"`
			} `json:"data"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			t.Fatalf("decode webhook: %v", err)
		}
		if payload.EventType == "ticket_archived" {
			archivedEvents[payload.Data.Ticket.ID] = true
		}
	}
	if len(archivedEvents) != 3 || !archivedEvents[draft] || !archivedEvents[inProgress] || !archivedEvents[rejected] {
		t.Fatalf("ticket_archived events = %v, want draft, in-progress and rejected tickets", archivedEvents)
	}
}

func TestSuspendUserKeepsTicketsAndAssignments(t *testing.T) {
	env := testutil.New(t)
	ticket := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	managerAssignments := outstandingAssignments(t, env, "sales-manager")
	if managerAssignments == 0 {
		t.Fatal("precondition: sales-manager has no outstanding assignment")
	}
	for _, code := range []string{"sales-rep", "sales-manager"} {
		if res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+env.UserID(t, code)+"/suspend", nil); res.Status != http.StatusOK {
			t.Fatalf("suspend %s: %d %s", code, res.Status, res.Body)
		}
	}
	if got := ticketStatus(t, env, ticket); got != "in_progress" {
		t.Fatalf("suspension changed ticket status to %s", got)
	}
	if got := outstandingAssignments(t, env, "sales-manager"); got != managerAssignments {
		t.Fatalf("suspension changed assignments: %d -> %d", managerAssignments, got)
	}
}

func TestDeleteUserRemovesOutstandingApproverAssignments(t *testing.T) {
	env := testutil.New(t)
	ticket := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if outstandingAssignments(t, env, "sales-manager") == 0 {
		t.Fatal("precondition: sales-manager has no outstanding assignment")
	}
	if res := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+env.UserID(t, "sales-manager"), nil); res.Status != http.StatusOK {
		t.Fatalf("delete approver: %d %s", res.Status, res.Body)
	}
	if got := outstandingAssignments(t, env, "sales-manager"); got != 0 {
		t.Fatalf("deleted approver still has %d outstanding assignments", got)
	}
	// The applicant's ticket is not archived because only its approver was deleted.
	if got := ticketStatus(t, env, ticket); got != "in_progress" {
		t.Fatalf("ticket status = %s, want in_progress", got)
	}
	// Completed approvals stay as history.
	var completed int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM ticket_assignees`).Scan(&completed); err != nil || completed == 0 {
		t.Fatalf("other assignments were removed too: count=%d err=%v", completed, err)
	}
}
