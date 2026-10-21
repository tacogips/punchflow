package v1_test

import (
	"fmt"
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

const pf22ExpenseWorkflow = "1f46223b-a6ef-4ee8-884e-d3f1e6b9f13b"
const pf22AmountField = "6f32e53a-d4ce-4d19-8723-754749c7d803"
const pf22PurposeField = "36b5a79c-d51f-45ad-8fab-24fcc9948789"
const pf22ExpenseDateField = "d10e07d0-0f87-465e-8d5f-c81b9fff9beb"
const pf22CostCenterField = "6eaffa72-f1cd-4a9f-bb17-4dec8b5449f9"
const pf22ReceiptField = "12547c53-e398-4b25-b9e2-d90230ba1abe"
const pf22ApproverNoteField = "95427a69-4c2c-472e-8799-7653cf1f36cc"

func pf22CreateTicket(t *testing.T, env *testutil.Env, persona testutil.Persona, status string, amount int) string {
	t.Helper()
	teamCode := "sales"
	if persona == testutil.Finance {
		teamCode = "finance"
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", teamCode)
	if err != nil {
		t.Fatalf("resolve %s team: %v", teamCode, err)
	}
	body := map[string]any{
		"status": status, "workflowId": pf22ExpenseWorkflow, "authorTeamId": teamID,
		"title": "PF-22 test ticket", "inputs": []map[string]any{
			{"formFieldId": pf22AmountField, "value": amount},
			{"formFieldId": pf22PurposeField, "value": "Integration test"},
			{"formFieldId": pf22ExpenseDateField, "value": "2026-01-01"},
			{"formFieldId": pf22CostCenterField, "generalMasterItemId": "00000000-0000-4000-8000-000000000041"},
			{"formFieldId": pf22ReceiptField, "files": []string{}},
			{"formFieldId": pf22ApproverNoteField, "value": ""},
		},
	}
	res := env.Do(t, persona, http.MethodPost, "/v1/tickets", body)
	if res.Status != http.StatusOK {
		t.Fatalf("create ticket: status=%d body=%s", res.Status, res.Body)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	res.JSON(t, &out)
	if out.ID == "" || out.Status != status {
		t.Fatalf("created ticket = %+v", out)
	}
	return out.ID
}

func TestTicketCreateGetAndVisibility(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	pf22CreateTicket(t, env, testutil.Finance, "draft", 5000)
	got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+id, nil)
	if got.Status != http.StatusOK {
		t.Fatalf("get ticket: %d %s", got.Status, got.Body)
	}
	var detail map[string]any
	got.JSON(t, &detail)
	for _, key := range []string{"id", "status", "workflow", "inputs", "steps", "author", "authorTeam"} {
		if _, ok := detail[key]; !ok {
			t.Errorf("ticket detail missing %q: %v", key, detail)
		}
	}
	list := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets?status=draft", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list tickets: %d %s", list.Status, list.Body)
	}
	for header, want := range map[string]string{"Page": "1", "Per-Page": "25", "Total": "1"} {
		if got := list.Header.Get(header); got != want {
			t.Errorf("%s header = %q, want %q", header, got, want)
		}
	}
	var rows []map[string]any
	list.JSON(t, &rows)
	if len(rows) != 1 || rows[0]["id"] != id {
		t.Fatalf("ticket list = %v", rows)
	}
	for _, query := range []string{"?status=invalid", "?status=draft&status=in_progress"} {
		testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets"+query, nil), http.StatusBadRequest, "invalid_parameter")
	}
	unknown := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/00000000-0000-4000-8000-000000000000", nil)
	testutil.AssertError(t, unknown, http.StatusNotFound, "resource_not_found")
}

func TestLinkedTicketCountCountsReciprocalRelationshipOnce(t *testing.T) {
	env := testutil.New(t)
	first := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	second := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_links(ticket_id,linked_ticket_id,created_at) VALUES(?,?,0),(?,?,0)`, first, second, second, first); err != nil {
		t.Fatal(err)
	}
	res := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+first, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("get linked ticket: status=%d body=%s", res.Status, res.Body)
	}
	var ticket map[string]any
	res.JSON(t, &ticket)
	if ticket["linkedTicketsCount"] != float64(1) {
		t.Fatalf("linkedTicketsCount=%v, want 1", ticket["linkedTicketsCount"])
	}
}

func TestTasksPendingAndAssignedToCurrentUser(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	unheldID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_assignees SET pending=1 WHERE ticket_id=? AND completed=0`, id); err != nil {
		t.Fatalf("hold first task: %v", err)
	}
	for _, tc := range []struct {
		query string
		want  map[string]bool
	}{
		{query: "", want: map[string]bool{id: true, unheldID: true}},
		{query: "?pending=true", want: map[string]bool{id: true}},
		{query: "?pending=false", want: map[string]bool{unheldID: true}},
	} {
		response := env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks"+tc.query, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("tasks%s: %d %s", tc.query, response.Status, response.Body)
		}
		var rows []struct {
			ID string `json:"id"`
		}
		response.JSON(t, &rows)
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.ID] = true
		}
		if len(got) != len(tc.want) {
			t.Errorf("tasks%s returned %v, want %v", tc.query, got, tc.want)
		}
		for wantID := range tc.want {
			if !got[wantID] {
				t.Errorf("tasks%s omitted %s: %v", tc.query, wantID, got)
			}
		}
		if total := response.Header.Get("Total"); total != fmt.Sprint(len(tc.want)) {
			t.Errorf("tasks%s Total=%q, want %d", tc.query, total, len(tc.want))
		}
	}
	invalid := env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?status=archived", nil)
	testutil.AssertError(t, invalid, http.StatusBadRequest, "invalid_parameter")
	if res := env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?status=in_progress", nil); res.Status != http.StatusOK {
		t.Fatalf("list tasks: %d %s", res.Status, res.Body)
	}
	var tasks []map[string]any
	env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?status=in_progress", nil).JSON(t, &tasks)
	found := false
	for _, task := range tasks {
		if task["id"] == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("manager task list omitted ticket %s: %v", id, tasks)
	}
	env.CallerFor = "sales-deputy"
	proxyTasks := env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/tasks?status=in_progress", nil)
	env.CallerFor = ""
	if proxyTasks.Status != http.StatusOK {
		t.Fatalf("list proxy tasks: %d %s", proxyTasks.Status, proxyTasks.Body)
	}
	var proxyRows []map[string]any
	proxyTasks.JSON(t, &proxyRows)
	proxyFound := false
	for _, task := range proxyRows {
		if task["id"] == id {
			proxyFound = true
		}
	}
	if !proxyFound {
		t.Fatalf("proxy task list omitted ticket %s: %v", id, proxyRows)
	}

	firstCreated := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	secondCreated := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	managerID, err := store.ResolveID(t.Context(), env.DB.Q(), "users", "sales-manager")
	if err != nil {
		t.Fatalf("resolve sales manager: %v", err)
	}
	for _, update := range []struct {
		ticketID string
		at       int64
	}{{firstCreated, 2_000}, {secondCreated, 1_000}} {
		if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_assignees SET assigned_at=? WHERE ticket_id=? AND user_id=?`, update.at, update.ticketID, managerID); err != nil {
			t.Fatalf("set assignment time: %v", err)
		}
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,pending,assigned_at,created_at,updated_at) SELECT ?,ticket_step_id,ticket_id,user_id,1,0,500,created_at,updated_at FROM ticket_assignees WHERE ticket_id=? AND user_id=? LIMIT 1`, store.NewID(), firstCreated, managerID); err != nil {
		t.Fatalf("insert earlier completed assignment: %v", err)
	}
	sorted := env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?assignedToMeAtStart=1970-01-01T00:00:00.000Z&assignedToMeAtEnd=1970-01-01T00:00:03.000Z&sortBy=assignedToMeAt-asc", nil)
	if sorted.Status != http.StatusOK {
		t.Fatalf("sort tasks by assignment time: %d %s", sorted.Status, sorted.Body)
	}
	var sortedRows []map[string]any
	sorted.JSON(t, &sortedRows)
	if len(sortedRows) < 2 || sortedRows[0]["id"] != secondCreated || sortedRows[1]["id"] != firstCreated {
		t.Fatalf("assignedToMeAt ascending order = %v; want %s before %s", sortedRows, secondCreated, firstCreated)
	}
	// Both bounds must qualify the same assignment row. The matching ticket
	// has one assignment before and one after the requested interval.
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_assignees SET assigned_at=1000 WHERE ticket_id=? AND user_id=?`, firstCreated, managerID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,pending,assigned_at,created_at,updated_at) SELECT ?,ticket_step_id,ticket_id,user_id,0,0,10000,created_at,updated_at FROM ticket_assignees WHERE ticket_id=? AND user_id=? LIMIT 1`, store.NewID(), firstCreated, managerID); err != nil {
		t.Fatal(err)
	}
	outside := env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?status=in_progress&assignedToMeAtStart=1970-01-01T00:00:04.000Z&assignedToMeAtEnd=1970-01-01T00:00:06.000Z", nil)
	var outsideRows []map[string]any
	outside.JSON(t, &outsideRows)
	for _, row := range outsideRows {
		if row["id"] == firstCreated {
			t.Fatalf("ticket with assignments only outside the interval matched: %v", outsideRows)
		}
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE users SET status='suspended' WHERE id=?`, managerID); err != nil {
		t.Fatal(err)
	}
	env.CallerFor = "sales-deputy"
	proxyTasks = env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/tasks?status=in_progress", nil)
	env.CallerFor = ""
	if proxyTasks.Status != http.StatusOK {
		t.Fatalf("suspended-principal proxy task list: %d %s", proxyTasks.Status, proxyTasks.Body)
	}
	proxyTasks.JSON(t, &proxyRows)
	for _, task := range proxyRows {
		if task["id"] == id {
			t.Fatalf("task list retained ticket %s after its principal was suspended", id)
		}
	}
}

func TestTicketAuthorizationGuards(t *testing.T) {
	env := testutil.New(t)
	employeeID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	financeID := pf22CreateTicket(t, env, testutil.Finance, "draft", 5000)

	testutil.AssertError(t,
		env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+financeID, nil),
		http.StatusForbidden, "invisible_ticket")

	adminList := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?status=draft", nil)
	if adminList.Status != http.StatusOK {
		t.Fatalf("admin list tickets: %d %s", adminList.Status, adminList.Body)
	}
	var rows []map[string]any
	adminList.JSON(t, &rows)
	if total := adminList.Header.Get("Total"); total == "" || total == "0" || total == "1" {
		t.Fatalf("admin Total header = %q, want at least two tickets", total)
	}
	found := map[string]bool{}
	for _, row := range rows {
		if id, ok := row["id"].(string); ok {
			found[id] = true
		}
	}
	if !found[employeeID] || !found[financeID] {
		t.Fatalf("admin list omitted employee or finance ticket: %v", rows)
	}
}

func TestTicketUpdateFormHTTP(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	patched := env.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+id+"/form", map[string]any{
		"inputs": []map[string]any{{"formFieldId": pf22PurposeField, "value": "Updated purpose"}},
	})
	if patched.Status != http.StatusOK {
		t.Fatalf("update ticket form: %d %s", patched.Status, patched.Body)
	}
	var detail map[string]any
	patched.JSON(t, &detail)
	inputs, ok := detail["inputs"].([]any)
	if !ok {
		t.Fatalf("updated ticket inputs have type %T", detail["inputs"])
	}
	updated := false
	for _, raw := range inputs {
		input, ok := raw.(map[string]any)
		if ok && input["formFieldId"] == pf22PurposeField && input["value"] == "Updated purpose" {
			updated = true
		}
	}
	if !updated {
		t.Fatalf("updated purpose missing from ticket inputs: %v", inputs)
	}
	testutil.AssertError(t,
		env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+id+"/form", map[string]any{}),
		http.StatusBadRequest, "invalid_parameter")
}

func TestTicketLinksHTTP(t *testing.T) {
	env := testutil.New(t)
	visibleID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	hiddenID := pf22CreateTicket(t, env, testutil.Finance, "draft", 5000)

	links := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+visibleID+"/links", nil)
	if links.Status != http.StatusOK {
		t.Fatalf("list ticket links: %d %s", links.Status, links.Body)
	}
	var rows []any
	links.JSON(t, &rows)
	if len(rows) != 0 {
		t.Fatalf("ticket links = %v, want empty array", rows)
	}
	for header, want := range map[string]string{"Page": "1", "Per-Page": "25", "Total": "0"} {
		if got := links.Header.Get(header); got != want {
			t.Errorf("%s header = %q, want %q", header, got, want)
		}
	}
	testutil.AssertError(t,
		env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+hiddenID+"/links", nil),
		http.StatusForbidden, "invisible_ticket")
	testutil.AssertError(t,
		env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/00000000-0000-4000-8000-000000000000/links", nil),
		http.StatusNotFound, "resource_not_found")
}
