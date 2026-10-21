package v1_test

import (
	"net/http"
	"sort"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

const n03MissingUser = "00000000-0000-4000-8000-00000000ffff"

func n03CreateBody(t *testing.T, env *testutil.Env, status string, extra map[string]any) map[string]any {
	t.Helper()
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatalf("resolve sales team: %v", err)
	}
	body := map[string]any{
		"status": status, "workflowId": pf22ExpenseWorkflow, "authorTeamId": teamID,
		"title": "N03 ticket", "inputs": []map[string]any{
			{"formFieldId": pf22AmountField, "value": 5000},
			{"formFieldId": pf22PurposeField, "value": "Inline viewers"},
			{"formFieldId": pf22ExpenseDateField, "value": "2026-01-01"},
			{"formFieldId": pf22CostCenterField, "generalMasterItemId": "00000000-0000-4000-8000-000000000041"},
			{"formFieldId": pf22ReceiptField, "files": []string{}},
			{"formFieldId": pf22ApproverNoteField, "value": ""},
		},
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

func n03ViewerUsers(t *testing.T, env *testutil.Env, ticketID string) []string {
	t.Helper()
	viewers, err := store.TicketViewers(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, viewer := range viewers {
		if viewer.UserID != nil {
			out = append(out, *viewer.UserID)
		}
	}
	sort.Strings(out)
	return out
}

func n03Sorted(ids ...string) []string {
	out := append([]string{}, ids...)
	sort.Strings(out)
	return out
}

func n03TicketCount(t *testing.T, env *testutil.Env) int {
	t.Helper()
	var count int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM tickets`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func n03Equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func n03AssertValidationField(t *testing.T, resp *testutil.Response, field string) {
	t.Helper()
	testutil.AssertError(t, resp, http.StatusUnprocessableEntity, "validation_failed")
	var body struct {
		Errors map[string][]string `json:"errors"`
	}
	resp.JSON(t, &body)
	if len(body.Errors[field]) == 0 {
		t.Fatalf("errors=%v, want an entry for %q", body.Errors, field)
	}
}

func TestCreateTicketPersistsInlineViewersAndWatcher(t *testing.T) {
	env := testutil.New(t)
	manager, finance := env.UserID(t, "sales-manager"), env.UserID(t, "finance-manager")
	resp := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{
		"viewers":       []map[string]any{{"userId": manager}, {"userId": manager}, {"userId": finance}},
		"watcherUserId": finance,
	}))
	if resp.Status != http.StatusOK {
		t.Fatalf("create: %d %s", resp.Status, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	resp.JSON(t, &created)
	if got := n03ViewerUsers(t, env, created.ID); !n03Equal(got, n03Sorted(manager, finance)) {
		t.Fatalf("viewers = %v, want the two distinct users", got)
	}
	state, err := store.TicketWatchState(t.Context(), env.DB.Q(), created.ID, finance)
	if err != nil || state != store.WatchStateWatching {
		t.Fatalf("watcher state = %q err=%v, want watching", state, err)
	}
	if state, err = store.TicketWatchState(t.Context(), env.DB.Q(), created.ID, manager); err != nil || state != "" {
		t.Fatalf("viewer-only user has watch state %q err=%v", state, err)
	}
	listed := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+created.ID+"/viewers", nil)
	if got := listed.Header.Get("Total"); got != "2" {
		t.Fatalf("viewer list Total = %q, want 2", got)
	}
}

func TestCreateTicketRejectsInvalidInlineUsers(t *testing.T) {
	env := testutil.New(t)
	manager, finance := env.UserID(t, "sales-manager"), env.UserID(t, "finance-manager")
	setStatus := func(userID, status string, deactivated bool) {
		t.Helper()
		if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE users SET status=?, deactivated_at=CASE WHEN ? THEN 1000 ELSE NULL END WHERE id=?`, status, deactivated, userID); err != nil {
			t.Fatal(err)
		}
	}
	before := n03TicketCount(t, env)

	for name, extra := range map[string]map[string]any{
		"nonexistent viewer":  {"viewers": []map[string]any{{"userId": n03MissingUser}}},
		"malformed viewer id": {"viewers": []map[string]any{{"userId": "not-a-uuid"}}},
		"missing viewer id":   {"viewers": []map[string]any{{}}},
	} {
		t.Run(name, func(t *testing.T) {
			n03AssertValidationField(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", extra)), "viewers[0].userId")
		})
	}
	t.Run("nonexistent watcher", func(t *testing.T) {
		n03AssertValidationField(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"watcherUserId": n03MissingUser})), "watcherUserId")
	})
	t.Run("deleted users", func(t *testing.T) {
		setStatus(finance, "deactivated", true)
		defer setStatus(finance, "activated", false)
		n03AssertValidationField(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"viewers": []map[string]any{{"userId": manager}, {"userId": finance}}})), "viewers[1].userId")
		n03AssertValidationField(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"watcherUserId": finance})), "watcherUserId")
	})
	if got := n03TicketCount(t, env); got != before {
		t.Fatalf("rejected requests created %d tickets", got-before)
	}

	t.Run("suspended users are accepted", func(t *testing.T) {
		setStatus(finance, "suspended", false)
		defer setStatus(finance, "activated", false)
		resp := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"viewers": []map[string]any{{"userId": finance}}, "watcherUserId": finance}))
		if resp.Status != http.StatusOK {
			t.Fatalf("suspended viewer/watcher: %d %s", resp.Status, resp.Body)
		}
	})
}

func TestUpdateTicketInlineViewersArePresenceAware(t *testing.T) {
	env := testutil.New(t)
	manager, finance, admin := env.UserID(t, "sales-manager"), env.UserID(t, "finance-manager"), env.UserID(t, "admin")
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	path := "/v1/tickets/" + ticketID

	patch := func(body map[string]any) *testutil.Response {
		return env.Do(t, testutil.Employee, http.MethodPatch, path, body)
	}
	if resp := patch(map[string]any{"viewers": []map[string]any{{"userId": manager}, {"userId": finance}}}); resp.Status != http.StatusOK {
		t.Fatalf("set viewers: %d %s", resp.Status, resp.Body)
	}
	if got := n03ViewerUsers(t, env, ticketID); !n03Equal(got, n03Sorted(manager, finance)) {
		t.Fatalf("viewers = %v", got)
	}

	// Absent fields leave viewers and watches untouched.
	if resp := patch(map[string]any{"title": "Renamed"}); resp.Status != http.StatusOK {
		t.Fatalf("title patch: %d %s", resp.Status, resp.Body)
	}
	if got := n03ViewerUsers(t, env, ticketID); !n03Equal(got, n03Sorted(manager, finance)) {
		t.Fatalf("viewers after unrelated patch = %v", got)
	}
	// An explicit null is not a replacement.
	if resp := patch(map[string]any{"viewers": nil, "watcherUserId": nil}); resp.Status != http.StatusOK {
		t.Fatalf("null patch: %d %s", resp.Status, resp.Body)
	}
	if got := n03ViewerUsers(t, env, ticketID); !n03Equal(got, n03Sorted(manager, finance)) {
		t.Fatalf("viewers after null patch = %v", got)
	}

	// A team viewer added through the dedicated endpoint survives replacement.
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	if resp := env.Do(t, testutil.Employee, http.MethodPost, path+"/viewers", map[string]any{"teamId": teamID}); resp.Status != http.StatusOK {
		t.Fatalf("team viewer: %d %s", resp.Status, resp.Body)
	}

	if resp := patch(map[string]any{"viewers": []map[string]any{{"userId": finance}, {"userId": admin}}, "watcherUserId": admin}); resp.Status != http.StatusOK {
		t.Fatalf("replace viewers: %d %s", resp.Status, resp.Body)
	}
	if got := n03ViewerUsers(t, env, ticketID); !n03Equal(got, n03Sorted(finance, admin)) {
		t.Fatalf("viewers after replacement = %v", got)
	}
	all, err := store.TicketViewers(t.Context(), env.DB.Q(), ticketID)
	if err != nil || len(all) != 3 {
		t.Fatalf("viewers incl. team = %d err=%v, want 2 users and the team", len(all), err)
	}
	if state, _ := store.TicketWatchState(t.Context(), env.DB.Q(), ticketID, admin); state != store.WatchStateWatching {
		t.Fatalf("watcher state = %q", state)
	}

	// Invalid references fail the whole request and change nothing.
	n03AssertValidationField(t, patch(map[string]any{"title": "Never saved", "viewers": []map[string]any{{"userId": n03MissingUser}}}), "viewers[0].userId")
	n03AssertValidationField(t, patch(map[string]any{"watcherUserId": n03MissingUser}), "watcherUserId")
	if got := n03ViewerUsers(t, env, ticketID); !n03Equal(got, n03Sorted(finance, admin)) {
		t.Fatalf("viewers after rejected patch = %v", got)
	}
	var title string
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT title FROM tickets WHERE id=?`, ticketID).Scan(&title); err != nil || title != "Renamed" {
		t.Fatalf("title after rejected patch = %q err=%v", title, err)
	}

	// An empty array clears the user viewers only.
	if resp := patch(map[string]any{"viewers": []map[string]any{}}); resp.Status != http.StatusOK {
		t.Fatalf("clear viewers: %d %s", resp.Status, resp.Body)
	}
	if got := n03ViewerUsers(t, env, ticketID); len(got) != 0 {
		t.Fatalf("viewers after clear = %v", got)
	}
	if all, err = store.TicketViewers(t.Context(), env.DB.Q(), ticketID); err != nil || len(all) != 1 {
		t.Fatalf("remaining viewers = %d err=%v, want the team viewer", len(all), err)
	}
}

func TestWatcherUserIDAlsoGrantsPrivateTicketVisibility(t *testing.T) {
	env := testutil.New(t)
	watcher := env.UserID(t, "finance-manager")
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": "in_progress", "workflowId": pf22ExpenseWorkflow, "authorTeamId": teamID,
		"title": "watcher sharing", "watcherUserId": watcher, "inputs": []map[string]any{
			{"formFieldId": pf22AmountField, "value": 5000},
			{"formFieldId": pf22PurposeField, "value": "watcher sharing"},
			{"formFieldId": pf22ExpenseDateField, "value": "2026-01-01"},
			{"formFieldId": pf22CostCenterField, "generalMasterItemId": "00000000-0000-4000-8000-000000000041"},
			{"formFieldId": pf22ReceiptField, "files": []string{}},
			{"formFieldId": pf22ApproverNoteField, "value": ""},
		},
	})
	if created.Status != http.StatusOK {
		t.Fatalf("create watcher ticket: %d %s", created.Status, created.Body)
	}
	var ticket struct {
		ID string `json:"id"`
	}
	created.JSON(t, &ticket)
	visible := env.Do(t, testutil.Finance, http.MethodGet, "/v1/tickets/"+ticket.ID, nil)
	if visible.Status != http.StatusOK {
		t.Fatalf("watcherUserId did not grant ticket visibility: %d %s", visible.Status, visible.Body)
	}
}

func TestInlineViewersRespectAllowEditingOfViewers(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=json_set(definition_json,'$.allowEditingOfViewers',json('false')) WHERE id=?`, workflowID); err != nil {
		t.Fatal(err)
	}
	manager := env.UserID(t, "sales-manager")
	resp := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"viewers": []map[string]any{{"userId": manager}}}))
	testutil.AssertError(t, resp, http.StatusForbidden, "missing_permission")
	// An empty list changes nothing, and a watcher is not a viewer edit.
	resp = env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", n03CreateBody(t, env, "draft", map[string]any{"viewers": []map[string]any{}, "watcherUserId": manager}))
	if resp.Status != http.StatusOK {
		t.Fatalf("create with empty viewers: %d %s", resp.Status, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	resp.JSON(t, &created)
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+created.ID, map[string]any{"viewers": []map[string]any{{"userId": manager}}}), http.StatusForbidden, "missing_permission")
}
