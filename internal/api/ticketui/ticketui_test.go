package ticketui_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func createTicket(t *testing.T, env *testutil.Env) string {
	return createTicketAmount(t, env, 5000)
}

func createTicketAmount(t *testing.T, env *testutil.Env, amount int) string {
	return createTicketStatus(t, env, "in_progress", amount)
}

func createTicketStatus(t *testing.T, env *testutil.Env, status string, amount int) string {
	t.Helper()
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			fields[field.Code] = field.ID
		}
	}
	response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": status, "workflowId": workflowID, "authorTeamId": teamID, "title": "ticket UI test",
		"inputs": []map[string]any{
			{"formFieldId": fields["amount"], "value": amount},
			{"formFieldId": fields["purpose"], "value": "ticket UI test"},
			{"formFieldId": fields["expense-date"], "value": "2026-01-01"},
			{"formFieldId": fields["cost-center"], "generalMasterItemId": itemID},
			{"formFieldId": fields["receipt"], "files": []string{}},
			{"formFieldId": fields["approver-note"], "value": ""},
		},
	})
	if response.Status != http.StatusOK {
		t.Fatalf("create ticket: %d %s", response.Status, response.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	response.JSON(t, &created)
	return created.ID
}

type ticketView struct {
	CurrentStep int            `json:"currentStep"`
	SubStatus   map[string]any `json:"subStatus"`
	Steps       []struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		StepOrder    int            `json:"stepOrder"`
		DisplayOrder int            `json:"displayOrder"`
		Status       string         `json:"status"`
		RouteStepID  *string        `json:"routeStepId"`
		Author       map[string]any `json:"author"`
	} `json:"steps"`
}

func fetch(t *testing.T, env *testutil.Env, id string) ticketView {
	t.Helper()
	response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets/"+id, nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get ticket: %d %s", response.Status, response.Body)
	}
	var view ticketView
	response.JSON(t, &view)
	return view
}

func auditCount(t *testing.T, env *testutil.Env, action, ticketID string) int {
	t.Helper()
	var n int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action=? AND ticket_id=?`, action, ticketID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestControlsEndpoint(t *testing.T) {
	env := testutil.New(t)
	id := createTicket(t, env)
	type controls struct {
		CancelApproval        bool  `json:"cancelApproval"`
		Raise                 bool  `json:"raise"`
		RaiseTargetStepOrders []int `json:"raiseTargetStepOrders"`
		AddCustomStep         bool  `json:"addCustomStep"`
	}
	get := func(persona testutil.Persona) controls {
		response := env.Do(t, persona, http.MethodGet, "/ui/api/tickets/"+id+"/controls", nil)
		if response.Status != http.StatusOK {
			t.Fatalf("controls: %d %s", response.Status, response.Body)
		}
		var out controls
		response.JSON(t, &out)
		return out
	}
	if got := get(testutil.Employee); !got.AddCustomStep || got.CancelApproval || got.Raise {
		t.Fatalf("applicant controls = %+v, want only custom steps", got)
	}
	if got := get(testutil.Finance); !got.Raise || len(got.RaiseTargetStepOrders) != 1 || got.RaiseTargetStepOrders[0] != 2 {
		t.Fatalf("finance controls = %+v, want raise to step 2", got)
	}
	testutil.AssertError(t, env.Do(t, "", http.MethodGet, "/ui/api/tickets/"+id+"/controls", nil), http.StatusUnauthorized, "invalid_access_token")
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodGet, "/ui/api/tickets/00000000-0000-4000-8000-000000000000/controls", nil), http.StatusNotFound, "resource_not_found")
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/unknown", nil), http.StatusNotFound, "endpoint_not_found")
}

func TestApplicationTeamsAndWorkflowMasterItemsAreAvailableToApplicants(t *testing.T) {
	env := testutil.New(t)
	teams := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/application-teams", nil)
	if teams.Status != http.StatusOK {
		t.Fatalf("application teams: %d %s", teams.Status, teams.Body)
	}
	var teamItems []struct {
		ID          string `json:"id"`
		ApproveOnly bool   `json:"approveOnly"`
	}
	teams.JSON(t, &teamItems)
	if len(teamItems) == 0 {
		t.Fatal("application teams are empty for an ordinary applicant")
	}
	for _, team := range teamItems {
		if team.ApproveOnly {
			t.Fatalf("application teams included approve-only team %s", team.ID)
		}
	}

	items := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/workflows/expense-claim/form-fields/cost-center/master-items", nil)
	if items.Status != http.StatusOK {
		t.Fatalf("workflow master items: %d %s", items.Status, items.Body)
	}
	var masterItems []struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	items.JSON(t, &masterItems)
	if len(masterItems) == 0 || masterItems[0].ID == "" {
		t.Fatalf("workflow master items = %+v, want populated selectable items", masterItems)
	}
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/workflows/expense-claim/form-fields/amount/master-items", nil), http.StatusNotFound, "resource_not_found")
	testutil.AssertError(t, env.Do(t, "", http.MethodGet, "/ui/api/application-teams", nil), http.StatusUnauthorized, "invalid_access_token")
}

func TestApplicationTeamsFollowAuthorizedProxyClient(t *testing.T) {
	env := testutil.New(t)
	proxyID, err := store.ResolveID(t.Context(), env.DB.Q(), "users", "sales-rep")
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := store.ResolveID(t.Context(), env.DB.Q(), "users", "ceo")
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE proxy_applicants SET proxy_user_id=?,workflow_ids=? WHERE user_id=?`, proxyID, `["`+workflowID+`"]`, clientID); err != nil {
		t.Fatal(err)
	}
	teams := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/application-teams?proxyClientUserId=ceo&workflowId=expense-claim", nil)
	if teams.Status != http.StatusOK {
		t.Fatalf("proxy application teams: %d %s", teams.Status, teams.Body)
	}
	var got []struct {
		ID string `json:"id"`
	}
	teams.JSON(t, &got)
	want, _, err := store.UserTeamIDs(t.Context(), env.DB.Q(), clientID, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("proxy team count=%d want %d", len(got), len(want))
	}
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodGet, "/ui/api/application-teams?proxyClientUserId=ceo&workflowId=expense-claim", nil), http.StatusForbidden, "missing_permission")
}

func TestWorkflowMasterItemPickerHidesExpiredItems(t *testing.T) {
	env := testutil.New(t)
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	expiredID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE general_master_items SET ends_on='2020-01-01' WHERE id=?`, expiredID); err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/workflows/expense-claim/form-fields/cost-center/master-items", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("master items: %d %s", response.Status, response.Body)
	}
	var items []struct {
		ID string `json:"id"`
	}
	response.JSON(t, &items)
	for _, item := range items {
		if item.ID == expiredID {
			t.Fatalf("expired item %s remains selectable", expiredID)
		}
	}
}

func TestCancelApprovalEndpoint(t *testing.T) {
	env := testutil.New(t)
	id := createTicket(t, env)
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodPost, "/ui/api/tickets/"+id+"/cancel-approval", nil), http.StatusUnprocessableEntity, "invalid_state")
	if response := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); response.Status != http.StatusOK {
		t.Fatalf("approve: %d %s", response.Status, response.Body)
	}
	if got := fetch(t, env, id); got.CurrentStep != 2 || got.SubStatus["code"] != "finance-check" {
		t.Fatalf("after approval: step %d sub-status %v", got.CurrentStep, got.SubStatus)
	}
	response := env.Do(t, testutil.Manager, http.MethodPost, "/ui/api/tickets/"+id+"/cancel-approval", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("cancel approval: %d %s", response.Status, response.Body)
	}
	var view ticketView
	response.JSON(t, &view)
	if view.CurrentStep != 1 || view.SubStatus["code"] != "under-review" {
		t.Fatalf("after cancel: step %d sub-status %v", view.CurrentStep, view.SubStatus)
	}
	if n := auditCount(t, env, "ticket.cancel_approval", id); n != 1 {
		t.Fatalf("cancel audit logs = %d, want 1", n)
	}
	if response = env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); response.Status != http.StatusOK {
		t.Fatalf("approve again: %d %s", response.Status, response.Body)
	}
}

func TestSubmitAndCancelApprovalAcceptUIComments(t *testing.T) {
	env := testutil.New(t)
	draftID := createTicketStatus(t, env, "draft", 5000)
	submissionComment := "Submitted with this note"
	response := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+draftID+"/submit", map[string]any{"comment": submissionComment})
	if response.Status != http.StatusOK {
		t.Fatalf("submit with comment: %d %s", response.Status, response.Body)
	}
	var stored string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT body FROM comments WHERE ticket_id=?`, draftID).Scan(&stored); err != nil || stored != submissionComment {
		t.Fatalf("submission comment=%q err=%v", stored, err)
	}

	activeID := createTicket(t, env)
	if response = env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+activeID+"/approve", nil); response.Status != http.StatusOK {
		t.Fatalf("approve before cancellation: %d %s", response.Status, response.Body)
	}
	cancelComment := "Reopen with this note"
	response = env.Do(t, testutil.Manager, http.MethodPost, "/ui/api/tickets/"+activeID+"/cancel-approval", map[string]any{"comment": cancelComment})
	if response.Status != http.StatusOK {
		t.Fatalf("cancel with comment: %d %s", response.Status, response.Body)
	}
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT body FROM comments WHERE ticket_id=?`, activeID).Scan(&stored); err != nil || stored != cancelComment {
		t.Fatalf("cancellation comment=%q err=%v", stored, err)
	}
}

func TestRaiseEndpoint(t *testing.T) {
	env := testutil.New(t)
	env.RegisterReceiverWebhook(t, "secret")
	id := createTicket(t, env)
	var outboxBefore int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_events WHERE ticket_id=?`, id).Scan(&outboxBefore); err != nil {
		t.Fatal(err)
	}
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodPost, "/ui/api/tickets/"+id+"/raise", nil), http.StatusUnprocessableEntity, "invalid_state")
	response := env.Do(t, testutil.Finance, http.MethodPost, "/ui/api/tickets/"+id+"/raise", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("raise: %d %s", response.Status, response.Body)
	}
	var view ticketView
	response.JSON(t, &view)
	if view.CurrentStep != 2 {
		t.Fatalf("current step = %d, want 2", view.CurrentStep)
	}
	statuses := map[string]string{}
	for _, step := range view.Steps {
		statuses[step.Title] = step.Status
	}
	if statuses["Manager approval"] != "raised" {
		t.Fatalf("step statuses = %v, want the first step raised", statuses)
	}
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil), http.StatusForbidden, "user_not_assigned")
	if n := auditCount(t, env, "ticket.raise", id); n != 1 {
		t.Fatalf("raise audit logs = %d, want 1", n)
	}
	var outboxAfter int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_events WHERE ticket_id=?`, id).Scan(&outboxAfter); err != nil {
		t.Fatal(err)
	}
	if outboxAfter != outboxBefore {
		t.Fatalf("raise webhook outbox events changed from %d to %d, want none added", outboxBefore, outboxAfter)
	}
	var raiseWebhooks int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_events WHERE ticket_id=? AND event_type='ticket_updated'`, id).Scan(&raiseWebhooks); err != nil {
		t.Fatal(err)
	}
	if raiseWebhooks != 0 {
		t.Fatalf("raise ticket_updated outbox rows=%d, want 0", raiseWebhooks)
	}
	var requestNotifications int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM notifications WHERE ticket_id=? AND user_id=? AND kind='approval_requested'`, id, env.UserID(t, "finance-manager")).Scan(&requestNotifications); err != nil {
		t.Fatal(err)
	}
	if requestNotifications != 1 {
		t.Fatalf("next approver request notifications=%d, want 1", requestNotifications)
	}
}

func TestCustomStepEndpoints(t *testing.T) {
	env := testutil.New(t)
	id := createTicket(t, env)
	hr := env.UserID(t, "hr-manager")
	body := map[string]any{"title": "Legal check", "actionType": "approve", "userIds": []string{hr}}
	testutil.AssertError(t, env.Do(t, testutil.Finance, http.MethodPost, "/ui/api/tickets/"+id+"/custom-steps", body), http.StatusForbidden, "missing_permission")
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+id+"/custom-steps", map[string]any{"title": "", "actionType": "approve", "userIds": []string{hr}}), http.StatusUnprocessableEntity, "validation_failed")
	response := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+id+"/custom-steps", body)
	if response.Status != http.StatusOK {
		t.Fatalf("add custom step: %d %s", response.Status, response.Body)
	}
	var view ticketView
	response.JSON(t, &view)
	if len(view.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(view.Steps))
	}
	custom := view.Steps[1]
	if custom.Title != "Legal check" || custom.RouteStepID != nil || custom.Author == nil || custom.StepOrder != 2 || view.Steps[2].StepOrder != 3 {
		t.Fatalf("custom step layout: %+v", view.Steps)
	}
	if n := auditCount(t, env, "ticket.add_custom_step", id); n != 1 {
		t.Fatalf("add audit logs = %d, want 1", n)
	}
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodDelete, "/ui/api/tickets/"+id+"/custom-steps/"+custom.ID, nil), http.StatusForbidden, "missing_permission")
	removed := env.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+id+"/custom-steps/"+custom.ID, nil)
	if removed.Status != http.StatusOK {
		t.Fatalf("remove custom step: %d %s", removed.Status, removed.Body)
	}
	if got := fetch(t, env, id); len(got.Steps) != 2 {
		t.Fatalf("steps after removal = %d, want 2", len(got.Steps))
	}
}

func TestSubStatusListEndpoint(t *testing.T) {
	env := testutil.New(t)
	response := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/sub-statuses", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("list sub-statuses: %d %s", response.Status, response.Body)
	}
	var items []struct {
		Code            string `json:"code"`
		AllowWithdrawal bool   `json:"allowWithdrawal"`
	}
	response.JSON(t, &items)
	if len(items) != 2 || items[0].Code != "finance-check" || items[0].AllowWithdrawal {
		t.Fatalf("sub-statuses = %+v", items)
	}
}

func TestAssignCurrentApproverAddsDistinctUsersToAssignedCurrentStep(t *testing.T) {
	env := testutil.New(t)
	ticketID := createTicket(t, env)
	env.RegisterReceiverWebhook(t, "r6-assignee-updatedAt")
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET updated_at=1000 WHERE id=?`, ticketID); err != nil {
		t.Fatal(err)
	}
	steps, err := store.EngTicketSteps(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	var suspendedAssigneeID string
	var activeStepID string
	for _, step := range steps {
		if step.StepOrder == 1 && step.Status == "in_progress" {
			activeStepID = step.ID
			// A suspended original assignee remains attached while an active
			// replacement is added (Help Center articles 28410770323737 and
			// 43509621580313; INF-NUM-06).
			if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT user_id FROM ticket_assignees WHERE ticket_step_id=? LIMIT 1`, step.ID).Scan(&suspendedAssigneeID); err != nil {
				t.Fatal(err)
			}
			if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE users SET status='suspended' WHERE id=?`, suspendedAssigneeID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if activeStepID == "" {
		t.Fatal("active step not found")
	}
	for _, userCode := range []string{"accountant", "ceo"} {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+ticketID+"/assignees", map[string]any{"ticketStepId": activeStepID, "userId": env.UserID(t, userCode)})
		if response.Status != http.StatusOK {
			t.Fatalf("assign current approver %s status=%d body=%s", userCode, response.Status, response.Body)
		}
	}
	var persistedUpdatedAt int64
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT updated_at FROM tickets WHERE id=?`, ticketID).Scan(&persistedUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if persistedUpdatedAt <= 1000 {
		t.Fatalf("admin-added approver persisted updated_at=%d, want newer than 1000", persistedUpdatedAt)
	}
	var payload []byte
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT body FROM webhook_events WHERE ticket_id=? AND event_type='ticket_updated' ORDER BY rowid DESC LIMIT 1`, ticketID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err = json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode ticket_updated payload: %v", err)
	}
	ticket := event["data"].(map[string]any)["ticket"].(map[string]any)
	if ticket["updatedAt"] == nil || ticket["updatedAt"] == "1970-01-01T09:00:01.000+09:00" {
		t.Fatalf("ticket_updated payload did not carry updated timestamp: %v", ticket["updatedAt"])
	}
	addedUserID := env.UserID(t, "ceo")
	foundAddedUser := false
	for _, rawStep := range ticket["steps"].([]any) {
		step := rawStep.(map[string]any)
		for _, rawAssignee := range step["assignees"].([]any) {
			assignee := rawAssignee.(map[string]any)
			if user, ok := assignee["user"].(map[string]any); ok && user["id"] == addedUserID {
				foundAddedUser = true
			}
		}
	}
	if !foundAddedUser {
		t.Fatalf("ticket_updated payload omitted added approver %s; steps=%#v", addedUserID, ticket["steps"])
	}
	steps, err = store.EngTicketSteps(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.StepOrder == 1 && step.Status == "in_progress" {
			assignees, er := store.EngStepAssignees(t.Context(), env.DB.Q(), step.ID)
			if er != nil {
				t.Fatal(er)
			}
			if len(assignees) != 3 {
				t.Fatalf("current assignees=%+v", assignees)
			}
			users := map[string]bool{}
			for _, assignee := range assignees {
				users[assignee.UserID] = true
			}
			for _, userCode := range []string{"accountant", "ceo"} {
				if !users[env.UserID(t, userCode)] {
					t.Errorf("current assignees omitted added user %s: %+v", userCode, assignees)
				}
			}
			if !users[suspendedAssigneeID] {
				t.Errorf("suspended existing assignee %s was removed: %+v", suspendedAssigneeID, assignees)
			}
			return
		}
	}
	t.Fatal("current step not found")
}

func TestAssignCurrentApproverFillsMultiApprovalStepIncrementally(t *testing.T) {
	env := testutil.New(t)
	ticketID := createTicket(t, env)
	steps, err := store.EngTicketSteps(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	var activeStepID string
	for _, step := range steps {
		if step.StepOrder == 1 && step.Status == "in_progress" {
			activeStepID = step.ID
			break
		}
	}
	if activeStepID == "" {
		t.Fatal("active current step not found")
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_steps SET required_approvals_number=2 WHERE id=?`, activeStepID); err != nil {
		t.Fatal(err)
	}
	if err = store.EngDeleteStepAssignees(t.Context(), env.DB.Q(), activeStepID); err != nil {
		t.Fatal(err)
	}
	for _, userCode := range []string{"accountant", "ceo"} {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+ticketID+"/assignees", map[string]any{"ticketStepId": activeStepID, "userId": env.UserID(t, userCode)})
		if response.Status != http.StatusOK {
			t.Fatalf("assign multi-approval user %s status=%d body=%s", userCode, response.Status, response.Body)
		}
	}
	assignees, err := store.EngStepAssignees(t.Context(), env.DB.Q(), activeStepID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 2 {
		t.Fatalf("multi-approval assignees=%+v, want both distinct users", assignees)
	}
}

func TestAssignCurrentApproverTargetsSelectedParallelStep(t *testing.T) {
	env := testutil.New(t)
	ticketID := createTicketAmount(t, env, 100000)
	if response := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+ticketID+"/approve", nil); response.Status != http.StatusOK {
		t.Fatalf("advance to parallel group: %d %s", response.Status, response.Body)
	}
	steps, err := store.EngTicketSteps(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	active := []string{}
	for _, step := range steps {
		if step.StepOrder == 2 && step.Status == "in_progress" {
			active = append(active, step.ID)
		}
	}
	if len(active) != 2 {
		t.Fatalf("active parallel steps=%v, want two", active)
	}
	response := env.Do(t, testutil.Admin, http.MethodPost, "/ui/api/tickets/"+ticketID+"/assignees", map[string]any{
		"ticketStepId": active[1], "userId": env.UserID(t, "engineer"),
	})
	if response.Status != http.StatusOK {
		t.Fatalf("assign selected parallel step: %d %s", response.Status, response.Body)
	}
	assignees, err := store.EngStepAssignees(t.Context(), env.DB.Q(), active[1])
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, assignee := range assignees {
		found = found || assignee.UserID == env.UserID(t, "engineer")
	}
	if !found {
		t.Fatalf("selected step %s assignees=%+v", active[1], assignees)
	}
}
