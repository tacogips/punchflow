package ticketui_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func createTicket(t *testing.T, env *testutil.Env) string {
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
		"status": "in_progress", "workflowId": workflowID, "authorTeamId": teamID, "title": "ticket UI test",
		"inputs": []map[string]any{
			{"formFieldId": fields["amount"], "value": 5000},
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

func TestRaiseEndpoint(t *testing.T) {
	env := testutil.New(t)
	id := createTicket(t, env)
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
