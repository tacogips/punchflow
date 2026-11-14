package uiapi_test

import (
	"net/http"
	"testing"

	"punchflow/internal/notify"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

type watchStatus struct {
	Watching bool   `json:"watching"`
	Source   string `json:"source"`
	OptedOut bool   `json:"optedOut"`
}

func (e *expenseEnv) watchStatus(t *testing.T, persona testutil.Persona, ticketID string) watchStatus {
	t.Helper()
	response := e.Do(t, persona, http.MethodGet, "/ui/api/tickets/"+ticketID+"/watch", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("watch status: %d %s", response.Status, response.Body)
	}
	var out watchStatus
	response.JSON(t, &out)
	return out
}

// NF-11: applicants and approvers watch automatically, an explicit opt-out is
// persisted, and the watch status agrees with who is actually notified.
func TestApproversWatchAutomaticallyAndMayOptOut(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Auto watch", purpose: "auto watch"})

	if got := e.watchStatus(t, testutil.Manager, id); !got.Watching || got.Source != "approver" {
		t.Fatalf("approver watch status = %+v, want automatic approver watching", got)
	}
	if got := e.watchStatus(t, testutil.Employee, id); !got.Watching || got.Source != "applicant" {
		t.Fatalf("applicant watch status = %+v, want automatic applicant watching", got)
	}

	// The first approver opts out; the author does too.
	if response := e.Do(t, testutil.Manager, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("manager unwatch: %d %s", response.Status, response.Body)
	}
	if response := e.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("author unwatch: %d %s", response.Status, response.Body)
	}
	if got := e.watchStatus(t, testutil.Manager, id); got.Watching || !got.OptedOut {
		t.Fatalf("opted-out approver status = %+v", got)
	}

	// The next approver is asked and watches; the opted-out users hear nothing.
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	if got := e.inbox(t, testutil.Employee, ""); len(got) != 0 {
		t.Fatalf("opted-out author was notified: %v", kinds(got))
	}
	if response := e.post(t, testutil.Finance, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("final approve: %d %s", response.Status, response.Body)
	}
	if got := e.inbox(t, testutil.Employee, ""); len(got) != 0 {
		t.Fatalf("opted-out author was notified of completion: %v", kinds(got))
	}
	manager := e.inbox(t, testutil.Manager, "")
	if countKind(manager, "ticket_completed", id) != 0 || countKind(manager, "ticket_approved", id) != 0 {
		t.Fatalf("opted-out approver was notified: %v", kinds(manager))
	}
	// Being asked to act is a task and is not affected by the opt-out.
	if countKind(manager, "approval_requested", id) != 1 {
		t.Fatalf("opted-out approver inbox = %v, want the original approval request", kinds(manager))
	}

	// A previous approver who did not opt out keeps hearing about the ticket.
	other := newExpenseEnv(t)
	second := other.createTicket(t, testutil.Employee, ticketSpec{title: "Follow along", purpose: "follow"})
	if response := other.post(t, testutil.Manager, "/v1/tickets/"+second+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	if response := other.post(t, testutil.Finance, "/v1/tickets/"+second+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("final approve: %d %s", response.Status, response.Body)
	}
	if got := other.inbox(t, testutil.Manager, ""); countKind(got, "ticket_completed", second) != 1 {
		t.Fatalf("earlier approver inbox = %v, want ticket_completed", kinds(got))
	}
	if got := other.inbox(t, testutil.Employee, ""); countKind(got, "ticket_completed", second) != 1 {
		t.Fatalf("author inbox = %v, want ticket_completed", kinds(got))
	}
}

// NF-11: the status endpoint and delivery use one calculation.
func TestWatchStatusMatchesEffectiveWatchers(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Same calculation", purpose: "calc"})
	if response := e.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin watch: %d %s", response.Status, response.Body)
	}
	if response := e.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("author unwatch: %d %s", response.Status, response.Body)
	}
	ticket, err := store.EngLoadTicket(t.Context(), e.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := notify.EffectiveWatchers(t.Context(), e.DB.Q(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	inEffective := map[string]bool{}
	for _, userID := range effective {
		inEffective[userID] = true
	}
	for code, persona := range map[string]testutil.Persona{"sales-rep": testutil.Employee, "sales-manager": testutil.Manager, "admin": testutil.Admin, "finance-manager": testutil.Finance} {
		userID := e.UserID(t, code)
		status, statusErr := notify.WatchStatusFor(t.Context(), e.DB.Q(), ticket, userID)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.Watching != inEffective[userID] {
			t.Errorf("%s: WatchStatusFor.Watching=%v but effective watcher=%v", persona, status.Watching, inEffective[userID])
		}
	}
	if inEffective[e.UserID(t, "sales-rep")] || !inEffective[e.UserID(t, "admin")] || !inEffective[e.UserID(t, "sales-manager")] {
		t.Fatalf("effective watchers = %v", effective)
	}
}

// NF-11: ticket_updated produces a local edit notification for watchers.
func TestTicketUpdatedNotifiesWatchers(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Edited", purpose: "edit"})
	if response := e.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin watch: %d %s", response.Status, response.Body)
	}
	response := e.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+id+"/form", map[string]any{
		"inputs": []map[string]any{{"formFieldId": e.fields["approver-note"], "value": "looks fine"}},
	})
	if response.Status != http.StatusOK {
		t.Fatalf("approver form edit: %d %s", response.Status, response.Body)
	}
	for _, persona := range []testutil.Persona{testutil.Employee, testutil.Admin} {
		if got := e.inbox(t, persona, ""); countKind(got, "ticket_updated", id) != 1 {
			t.Fatalf("%s inbox = %v, want one ticket_updated", persona, kinds(got))
		}
	}
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "ticket_updated", id) != 0 {
		t.Fatalf("editing approver was notified of own edit: %v", kinds(got))
	}
	// An opted-out watcher is not told about later edits.
	if response = e.Do(t, testutil.Admin, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin unwatch: %d %s", response.Status, response.Body)
	}
	response = e.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+id+"/form", map[string]any{
		"inputs": []map[string]any{{"formFieldId": e.fields["approver-note"], "value": "second thought"}},
	})
	if response.Status != http.StatusOK {
		t.Fatalf("second approver form edit: %d %s", response.Status, response.Body)
	}
	if got := e.inbox(t, testutil.Admin, ""); countKind(got, "ticket_updated", id) != 1 {
		t.Fatalf("unwatched admin inbox = %v, want the first edit only", kinds(got))
	}
	if got := e.inbox(t, testutil.Employee, ""); countKind(got, "ticket_updated", id) != 2 {
		t.Fatalf("author inbox = %v, want two ticket_updated", kinds(got))
	}
}
