package uiapi_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

// setWorkflowDefinition patches the expense-claim definition JSON.
func (e *expenseEnv) setWorkflowDefinition(t *testing.T, path, jsonValue string) {
	t.Helper()
	if _, err := e.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=json_set(definition_json,?,json(?)) WHERE id=?`, path, jsonValue, e.workflowID); err != nil {
		t.Fatalf("patch workflow definition: %v", err)
	}
}

func (e *expenseEnv) storedNotifications(t *testing.T, userCode string) []store.Notification {
	t.Helper()
	items, _, err := store.ListNotifications(t.Context(), e.DB.Q(), e.UserID(t, userCode), false, 0, 100)
	if err != nil {
		t.Fatalf("list notifications for %s: %v", userCode, err)
	}
	return items
}

func storedKinds(items []store.Notification, ticketID string) []string {
	out := []string{}
	for _, item := range items {
		if ticketID == "" || item.TicketID == ticketID {
			out = append(out, item.Kind)
		}
	}
	return out
}

func TestWorkflowLevelWatchersAutoSubscribe(t *testing.T) {
	e := newExpenseEnv(t)
	// A user watcher (admin, who can read every ticket) and a team watcher
	// (engineering, who can read the ticket because it is public).
	e.setWorkflowDefinition(t, "$.watchers", `[{"id":"w-user","watcherType":"user","userCode":"admin"},{"id":"w-team","watcherType":"team","teamCode":"engineering"}]`)
	e.setWorkflowDefinition(t, "$.publicTicket", `true`)

	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Watched by workflow", purpose: "wf watchers"})

	var state struct {
		Watching        bool   `json:"watching"`
		Source          string `json:"source"`
		WorkflowWatcher bool   `json:"workflowWatcher"`
	}
	response := e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/tickets/"+id+"/watch", nil)
	response.JSON(t, &state)
	if !state.Watching || state.Source != "workflow" || !state.WorkflowWatcher {
		t.Fatalf("workflow watcher state = %+v", state)
	}
	if response = e.Do(t, testutil.Employee, http.MethodGet, "/ui/api/tickets/"+id+"/watch", nil); response.Status != http.StatusOK {
		t.Fatalf("author watch state: %d", response.Status)
	}

	if response = e.post(t, testutil.Manager, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	if got := e.inbox(t, testutil.Admin, ""); countKind(got, "ticket_approved", id) != 1 {
		t.Fatalf("workflow user watcher inbox = %v", kinds(got))
	}
	for _, code := range []string{"engineer", "eng-manager"} {
		if got := storedKinds(e.storedNotifications(t, code), id); len(got) != 2 || got[0] != "ticket_approved" || got[1] != "ticket_applied" {
			t.Fatalf("workflow team watcher %s notifications = %v", code, got)
		}
	}
	if got := storedKinds(e.storedNotifications(t, "assistant"), id); len(got) != 0 {
		t.Fatalf("non-watcher received notifications: %v", got)
	}

	t.Run("opting out of a workflow watch is remembered", func(t *testing.T) {
		response := e.Do(t, testutil.Admin, http.MethodDelete, "/ui/api/tickets/"+id+"/watch", nil)
		response.JSON(t, &state)
		if state.Watching || !state.WorkflowWatcher {
			t.Fatalf("after opt out = %+v", state)
		}
		if response = e.post(t, testutil.Finance, "/v1/tickets/"+id+"/approve", map[string]any{}); response.Status != http.StatusOK {
			t.Fatalf("approve: %d %s", response.Status, response.Body)
		}
		if got := e.inbox(t, testutil.Admin, ""); countKind(got, "ticket_completed", id) != 0 {
			t.Fatalf("opted-out watcher received completion: %v", kinds(got))
		}
		if got := storedKinds(e.storedNotifications(t, "engineer"), id); len(got) != 4 {
			t.Fatalf("remaining watcher notifications = %v, want applied, approved, approved, completed", got)
		}
	})

	t.Run("a watcher who cannot view the ticket is not notified", func(t *testing.T) {
		other := newExpenseEnv(t)
		other.setWorkflowDefinition(t, "$.watchers", `[{"id":"w-team","watcherType":"team","teamCode":"engineering"}]`)
		hidden := other.createTicket(t, testutil.Employee, ticketSpec{title: "Not public", purpose: "private"})
		if response := other.post(t, testutil.Manager, "/v1/tickets/"+hidden+"/approve", map[string]any{}); response.Status != http.StatusOK {
			t.Fatalf("approve: %d %s", response.Status, response.Body)
		}
		if got := storedKinds(other.storedNotifications(t, "engineer"), hidden); len(got) != 0 {
			t.Fatalf("watcher without view access was notified: %v", got)
		}
	})
}
