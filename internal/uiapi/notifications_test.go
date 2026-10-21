package uiapi_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

func TestWatchToggle(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{status: "draft", title: "Watch me", purpose: "watch"})
	path := "/ui/api/tickets/" + id + "/watch"

	type watchState struct {
		Watching        bool   `json:"watching"`
		Source          string `json:"source"`
		WorkflowWatcher bool   `json:"workflowWatcher"`
		OptedOut        bool   `json:"optedOut"`
	}
	read := func(response *testutil.Response) watchState {
		t.Helper()
		if response.Status != http.StatusOK {
			t.Fatalf("watch request: status=%d body=%s", response.Status, response.Body)
		}
		var state watchState
		response.JSON(t, &state)
		return state
	}

	// The applicant watches automatically; stopping is an explicit opt-out.
	if got := read(e.Do(t, testutil.Employee, http.MethodGet, path, nil)); !got.Watching || got.Source != "applicant" || got.OptedOut {
		t.Fatalf("initial author watch state = %+v, want automatic applicant watching", got)
	}
	if got := read(e.Do(t, testutil.Employee, http.MethodDelete, path, nil)); got.Watching || !got.OptedOut {
		t.Fatalf("after author unwatch = %+v, want opted out", got)
	}
	if got := read(e.Do(t, testutil.Employee, http.MethodGet, path, nil)); got.Watching || !got.OptedOut {
		t.Fatalf("opt-out did not persist: %+v", got)
	}
	if got := read(e.Do(t, testutil.Employee, http.MethodPost, path, nil)); !got.Watching || got.Source != "applicant" || got.OptedOut {
		t.Fatalf("after author watch again = %+v, want applicant watching", got)
	}

	// A user without an automatic subscription needs an explicit one.
	if got := read(e.Do(t, testutil.Admin, http.MethodGet, path, nil)); got.Watching || got.OptedOut {
		t.Fatalf("initial admin watch state = %+v, want not watching", got)
	}
	if got := read(e.Do(t, testutil.Admin, http.MethodPost, path, nil)); !got.Watching || got.Source != "manual" {
		t.Fatalf("after admin watch = %+v, want manual watching", got)
	}
	if got := read(e.Do(t, testutil.Admin, http.MethodGet, path, nil)); !got.Watching {
		t.Fatalf("watch did not persist: %+v", got)
	}
	if got := read(e.Do(t, testutil.Admin, http.MethodDelete, path, nil)); got.Watching || got.OptedOut {
		t.Fatalf("after admin unwatch = %+v, want not watching and not an opt-out", got)
	}

	t.Run("invisible ticket cannot be watched", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, testutil.Finance, http.MethodPost, path, nil), http.StatusForbidden, "invisible_ticket")
	})
	t.Run("unknown ticket", func(t *testing.T) {
		missing := e.Do(t, testutil.Employee, http.MethodGet, "/ui/api/tickets/00000000-0000-4000-8000-000000000000/watch", nil)
		testutil.AssertError(t, missing, http.StatusNotFound, "resource_not_found")
	})
	t.Run("requires authentication", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, "", http.MethodGet, path, nil), http.StatusUnauthorized, "invalid_access_token")
	})
}

func TestNotificationLifecycleFanout(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Client dinner", purpose: "dinner"})

	// The first approver is told an approval is waiting; the author, who acted,
	// is not notified of their own application.
	manager := e.inbox(t, testutil.Manager, "")
	if countKind(manager, "approval_requested", id) != 1 {
		t.Fatalf("manager inbox = %v, want one approval_requested", kinds(manager))
	}
	if got := e.inbox(t, testutil.Employee, ""); len(got) != 0 {
		t.Fatalf("author inbox = %v, want empty (actor is not notified)", kinds(got))
	}
	if manager[0].TicketTitle == nil || *manager[0].TicketTitle != "Client dinner" || !manager[0].TicketVisible {
		t.Fatalf("notification ticket = %+v", manager[0])
	}
	if manager[0].Actor == nil || manager[0].Actor.Name == "" {
		t.Fatalf("notification actor = %+v", manager[0].Actor)
	}
	if e.unread(t, testutil.Manager) != 1 {
		t.Fatalf("manager unread count != 1")
	}

	// The admin watches the ticket and so hears about the approval; the author
	// hears about it too, the acting manager does not. The next approver is asked.
	if response := e.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin watch: %d %s", response.Status, response.Body)
	}
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	if got := e.inbox(t, testutil.Employee, ""); countKind(got, "ticket_approved", id) != 1 {
		t.Fatalf("author inbox after approval = %v", kinds(got))
	}
	if got := e.inbox(t, testutil.Admin, ""); countKind(got, "ticket_approved", id) != 1 {
		t.Fatalf("watcher inbox after approval = %v", kinds(got))
	}
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "ticket_approved", id) != 0 {
		t.Fatalf("acting approver was notified of own approval: %v", kinds(got))
	}
	finance := e.inbox(t, testutil.Finance, "")
	if countKind(finance, "approval_requested", id) != 1 {
		t.Fatalf("next approver inbox = %v, want approval_requested", kinds(finance))
	}

	// A watcher who stops watching no longer hears about later events.
	if response := e.Do(t, testutil.Admin, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin unwatch: %d %s", response.Status, response.Body)
	}
	if response := e.post(t, testutil.Finance, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("final approve: %d %s", response.Status, response.Body)
	}
	author := e.inbox(t, testutil.Employee, "")
	if countKind(author, "ticket_completed", id) != 1 || countKind(author, "ticket_approved", id) != 2 {
		t.Fatalf("author inbox after completion = %v", kinds(author))
	}
	if got := e.inbox(t, testutil.Admin, ""); countKind(got, "ticket_completed", id) != 0 {
		t.Fatalf("unwatched admin received completion: %v", kinds(got))
	}
}

func TestNotificationDenyRejectWithdraw(t *testing.T) {
	e := newExpenseEnv(t)

	denied := e.createTicket(t, testutil.Employee, ticketSpec{title: "Deny me", purpose: "deny"})
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+denied+"/deny", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("deny: %d %s", response.Status, response.Body)
	}
	withdrawn := e.createTicket(t, testutil.Employee, ticketSpec{title: "Withdraw me", purpose: "withdraw"})
	if response := e.post(t, testutil.Employee, "/v1/tickets/"+withdrawn+"/withdraw", nil); response.Status != http.StatusOK {
		t.Fatalf("withdraw: %d %s", response.Status, response.Body)
	}
	rejected := e.createTicket(t, testutil.Employee, ticketSpec{title: "Reject me", purpose: "reject"})
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+rejected+"/reject", map[string]any{"to": 0}); response.Status != http.StatusOK {
		t.Fatalf("reject: %d %s", response.Status, response.Body)
	}

	author := e.inbox(t, testutil.Employee, "")
	if countKind(author, "ticket_denied", denied) != 1 || countKind(author, "ticket_rejected", rejected) != 1 {
		t.Fatalf("author inbox = %v", kinds(author))
	}
	if countKind(author, "ticket_withdrawn", withdrawn) != 0 {
		t.Fatalf("author notified of own withdrawal: %v", kinds(author))
	}
	// Approvers watch automatically, so the pending approver hears about the
	// withdrawal.
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "ticket_withdrawn", withdrawn) != 1 {
		t.Fatalf("approver was not notified of withdrawal: %v", kinds(got))
	}
}

func TestNotificationComments(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Commented", purpose: "comments"})
	if response := e.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("admin watch: %d %s", response.Status, response.Body)
	}
	financeID := e.UserID(t, "finance-manager")
	if response := e.Do(t, testutil.Admin, http.MethodPost, "/v1/tickets/"+id+"/viewers", map[string]any{"userId": financeID}); response.Status != http.StatusOK {
		t.Fatalf("add viewer: %d %s", response.Status, response.Body)
	}

	body := "Please review <@" + financeID + "> and confirm the dinner receipt"
	if response := e.post(t, testutil.Manager, "/v1/tickets/"+id+"/comments", map[string]any{"body": body}); response.Status != http.StatusOK {
		t.Fatalf("comment: %d %s", response.Status, response.Body)
	}

	mention := e.inbox(t, testutil.Finance, "")
	if countKind(mention, "comment_mention", id) != 1 || countKind(mention, "comment_created", id) != 0 {
		t.Fatalf("mentioned user inbox = %v", kinds(mention))
	}
	var found notification
	for _, item := range mention {
		if item.Kind == "comment_mention" {
			found = item
		}
	}
	if strings.Contains(found.Excerpt, "<@") || !strings.Contains(found.Excerpt, "Please review @") {
		t.Fatalf("mention excerpt = %q, want mention markup replaced by a name", found.Excerpt)
	}
	if got := e.inbox(t, testutil.Employee, ""); countKind(got, "comment_created", id) != 1 {
		t.Fatalf("author inbox = %v, want comment_created", kinds(got))
	}
	if got := e.inbox(t, testutil.Admin, ""); countKind(got, "comment_created", id) != 1 {
		t.Fatalf("watcher inbox = %v, want comment_created", kinds(got))
	}
	if got := e.inbox(t, testutil.Manager, ""); countKind(got, "comment_created", id) != 0 || countKind(got, "comment_mention", id) != 0 {
		t.Fatalf("commenter notified of own comment: %v", kinds(got))
	}
}

func TestNotificationInboxReadState(t *testing.T) {
	e := newExpenseEnv(t)
	first := e.createTicket(t, testutil.Employee, ticketSpec{title: "One", purpose: "one"})
	second := e.createTicket(t, testutil.Employee, ticketSpec{title: "Two", purpose: "two"})
	items := e.inbox(t, testutil.Manager, "")
	if len(items) != 2 || items[0].TicketID != second || items[1].TicketID != first {
		t.Fatalf("manager inbox order = %+v, want newest first", items)
	}
	if e.unread(t, testutil.Manager) != 2 {
		t.Fatalf("unread != 2")
	}

	response := e.post(t, testutil.Manager, "/ui/api/notifications/"+items[0].ID+"/read", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("mark read: %d %s", response.Status, response.Body)
	}
	if e.unread(t, testutil.Manager) != 1 {
		t.Fatalf("unread after mark read != 1")
	}
	unread := e.inbox(t, testutil.Manager, "?unread=true")
	if len(unread) != 1 || unread[0].ID != items[1].ID {
		t.Fatalf("unread filter = %+v", unread)
	}
	if all := e.inbox(t, testutil.Manager, ""); len(all) != 2 || !all[0].Read || all[1].Read {
		t.Fatalf("read flags = %+v", all)
	}

	t.Run("another user's notification is not found", func(t *testing.T) {
		testutil.AssertError(t, e.post(t, testutil.Finance, "/ui/api/notifications/"+items[1].ID+"/read", nil), http.StatusNotFound, "resource_not_found")
		if e.unread(t, testutil.Manager) != 1 {
			t.Fatalf("another user changed the manager's read state")
		}
	})
	t.Run("invalid unread filter", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, testutil.Manager, http.MethodGet, "/ui/api/notifications?unread=maybe", nil), http.StatusBadRequest, "invalid_parameter")
	})
	t.Run("pagination headers", func(t *testing.T) {
		page := e.Do(t, testutil.Manager, http.MethodGet, "/ui/api/notifications?perPage=1", nil)
		testutil.AssertPagination(t, page, 1, 1, 2)
	})

	all := e.post(t, testutil.Manager, "/ui/api/notifications/read-all", nil)
	if all.Status != http.StatusOK || e.unread(t, testutil.Manager) != 0 {
		t.Fatalf("read-all: status=%d unread=%d", all.Status, e.unread(t, testutil.Manager))
	}
}

func TestNotificationHidesTicketUserCannotView(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{status: "draft", title: "Secret title", purpose: "secret"})
	financeID := e.UserID(t, "finance-manager")
	// Simulate a notification whose ticket the recipient has since lost access to.
	if _, err := e.DB.Q().ExecContext(t.Context(),
		`INSERT INTO notifications(id,user_id,kind,ticket_id,excerpt,created_at) VALUES('n-hidden',?,?,?,'',1)`,
		financeID, "comment_created", id); err != nil {
		t.Fatal(err)
	}
	items := e.inbox(t, testutil.Finance, "")
	if len(items) != 1 {
		t.Fatalf("finance inbox = %+v", items)
	}
	if items[0].TicketVisible || items[0].TicketTitle != nil {
		t.Fatalf("notification leaks a ticket the user cannot view: %+v", items[0])
	}
}
