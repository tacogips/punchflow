package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

// compatSetExpenseRoute replaces the steps of the route used by expense-claim.
func compatSetExpenseRoute(t *testing.T, env *testutil.Env, steps ...model.RouteStep) {
	t.Helper()
	for i := range steps {
		steps[i].ID = "compat-" + steps[i].Code
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE routes SET definition_json=json_set(definition_json,'$.steps',json(?)) WHERE code=(SELECT json_extract(definition_json,'$.routeCode') FROM workflows WHERE id=?)`, string(raw), pf22ExpenseWorkflow); err != nil {
		t.Fatal(err)
	}
}

func compatStep(code string, order int, action string, users ...string) model.RouteStep {
	return model.RouteStep{Code: code, StepOrder: order, Title: code, StepType: model.StepTypeUser, ActionType: action, UserCodes: users, ManagerSelectionType: model.ManagerSelectionClosest}
}

func compatEventTypes(t *testing.T, env *testutil.Env, ticketID string) []string {
	t.Helper()
	rows, err := env.DB.Q().QueryContext(t.Context(), `SELECT event_type FROM webhook_events WHERE ticket_id=? ORDER BY rowid`, ticketID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var eventType string
		if err = rows.Scan(&eventType); err != nil {
			t.Fatal(err)
		}
		out = append(out, eventType)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func compatAssertEvents(t *testing.T, env *testutil.Env, ticketID string, want ...string) {
	t.Helper()
	got := compatEventTypes(t, env, ticketID)
	if len(got) != len(want) {
		t.Fatalf("outbox events=%v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("outbox events=%v, want %v", got, want)
		}
	}
}

func compatTicketStatus(t *testing.T, env *testutil.Env, persona testutil.Persona, ticketID string) string {
	t.Helper()
	res := env.Do(t, persona, http.MethodGet, "/v1/tickets/"+ticketID, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("get ticket: %d %s", res.Status, res.Body)
	}
	var detail struct {
		Status string `json:"status"`
	}
	res.JSON(t, &detail)
	return detail.Status
}

// WH-01: a partial approval is announced as ticket_approved.
func TestPartialApprovalOutboxEvent(t *testing.T) {
	env := testutil.New(t)
	env.RegisterReceiverWebhook(t, "secret")
	quorum := compatStep("quorum", 1, model.ActionTypeApprove, "sales-manager", "finance-manager")
	quorum.RequiredApprovalsNumber = 2
	compatSetExpenseRoute(t, env, quorum)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)

	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); res.Status != http.StatusOK {
		t.Fatalf("first approval: %d %s", res.Status, res.Body)
	}
	compatAssertEvents(t, env, id, "ticket_created", "ticket_opened", "ticket_approved")
	if res := env.Do(t, testutil.Finance, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); res.Status != http.StatusOK {
		t.Fatalf("second approval: %d %s", res.Status, res.Body)
	}
	compatAssertEvents(t, env, id, "ticket_created", "ticket_opened", "ticket_approved", "ticket_approved", "ticket_completed")
}

// compatCreateTicket posts an expense-claim ticket and returns its ID and the
// status reported by the response (which can differ from the requested one when
// the route completes at submission).
func compatCreateTicket(t *testing.T, env *testutil.Env, persona testutil.Persona, status string) (string, string) {
	t.Helper()
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	res := env.Do(t, persona, http.MethodPost, "/v1/tickets", map[string]any{
		"status": status, "workflowId": pf22ExpenseWorkflow, "authorTeamId": teamID, "title": "compat",
		"inputs": []map[string]any{
			{"formFieldId": pf22AmountField, "value": "5000"},
			{"formFieldId": pf22PurposeField, "value": "Integration test"},
			{"formFieldId": pf22ExpenseDateField, "value": "2026-01-01"},
			{"formFieldId": pf22CostCenterField, "generalMasterItemId": "00000000-0000-4000-8000-000000000041"},
			{"formFieldId": pf22ReceiptField, "files": []string{}},
			{"formFieldId": pf22ApproverNoteField, "value": ""},
		},
	})
	if res.Status != http.StatusOK {
		t.Fatalf("create ticket: %d %s", res.Status, res.Body)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	res.JSON(t, &out)
	return out.ID, out.Status
}

// WH-02 / PF-01: tickets whose route needs no user action complete on
// submission (direct creation and draft submission) and emit ticket_completed.
func TestSubmissionWithoutUserActionCompletesAndEmitsEvent(t *testing.T) {
	env := testutil.New(t)
	env.RegisterReceiverWebhook(t, "secret")
	compatSetExpenseRoute(t, env, compatStep("fyi", 1, model.ActionTypeNone, "sales-manager"))

	direct, status := compatCreateTicket(t, env, testutil.Employee, "in_progress")
	if status != "completed" {
		t.Fatalf("direct create status=%s, want completed", status)
	}
	compatAssertEvents(t, env, direct, "ticket_created", "ticket_opened", "ticket_completed")

	draft, status := compatCreateTicket(t, env, testutil.Employee, "draft")
	if status != "draft" {
		t.Fatalf("draft status=%s", status)
	}
	res := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+draft, map[string]any{"status": "in_progress"})
	if res.Status != http.StatusOK {
		t.Fatalf("submit draft: %d %s", res.Status, res.Body)
	}
	if got := compatTicketStatus(t, env, testutil.Employee, draft); got != "completed" {
		t.Fatalf("submitted draft status=%s, want completed", got)
	}
	compatAssertEvents(t, env, draft, "ticket_created", "ticket_opened", "ticket_completed")
}

// R10 / PF-05: the web UI saves drafts with an explicit status draft.
func TestPatchDraftWithExplicitDraftStatus(t *testing.T) {
	env := testutil.New(t)
	id, _ := compatCreateTicket(t, env, testutil.Employee, "draft")
	res := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+id, map[string]any{"status": "draft", "title": "renamed"})
	if res.Status != http.StatusOK {
		t.Fatalf("save draft with status draft: %d %s", res.Status, res.Body)
	}
	var detail struct {
		Status string `json:"status"`
		Title  string `json:"title"`
	}
	res.JSON(t, &detail)
	if detail.Status != "draft" || detail.Title != "renamed" {
		t.Fatalf("saved draft = %+v", detail)
	}
	bad := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+id, map[string]any{"status": "rejected"})
	testutil.AssertError(t, bad, http.StatusUnprocessableEntity, "invalid_state")
}

// R05 / PF-04: the applicant supplies nominated approvers through
// PATCH /v1/tickets/{id} before submitting.
func TestApplicantNominatesApproversOverHTTP(t *testing.T) {
	env := testutil.New(t)
	min := 1
	nominee := model.RouteStep{Code: "nominee", StepOrder: 1, Title: "nominee", StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min}
	compatSetExpenseRoute(t, env, nominee)
	id, _ := compatCreateTicket(t, env, testutil.Employee, "draft")

	approvers := []map[string]any{{"routeStepCode": "nominee", "userId": []string{env.UserID(t, "finance-manager")}}}
	res := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+id, map[string]any{"approvers": approvers})
	if res.Status != http.StatusOK {
		t.Fatalf("applicant nominates approvers: %d %s", res.Status, res.Body)
	}
	submit := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+id, map[string]any{"status": "in_progress"})
	if submit.Status != http.StatusOK {
		t.Fatalf("submit with persisted nominations: %d %s", submit.Status, submit.Body)
	}
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); res.Status != http.StatusForbidden {
		t.Fatalf("non-nominated manager approve: %d %s", res.Status, res.Body)
	}
	if res := env.Do(t, testutil.Finance, http.MethodPost, "/v1/tickets/"+id+"/approve", nil); res.Status != http.StatusOK {
		t.Fatalf("nominated approver approve: %d %s", res.Status, res.Body)
	}

	other, _ := compatCreateTicket(t, env, testutil.Employee, "draft")
	missing := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+other, map[string]any{"status": "in_progress"})
	testutil.AssertError(t, missing, http.StatusUnprocessableEntity, "validation_failed")
}

// R02 / PF-03: a viewer added through the viewers API can read the ticket, see it
// in lists, and a public ticket is readable by everyone.
func TestTicketViewersAndPublicStatusGrantAccess(t *testing.T) {
	env := testutil.New(t)
	env.CallerFor = "engineer"
	id, _ := compatCreateTicket(t, env, testutil.Employee, "in_progress")
	detail := "/v1/tickets/" + id

	testutil.AssertError(t, env.Do(t, testutil.Integration, http.MethodGet, detail, nil), http.StatusForbidden, "invisible_ticket")
	if got := compatListedIDs(t, env, "/v1/tickets"); contains(got, id) {
		t.Fatalf("unrelated user lists the ticket before sharing: %v", got)
	}

	added := env.Do(t, testutil.Employee, http.MethodPost, detail+"/viewers", map[string]any{"userId": env.UserID(t, "engineer")})
	if added.Status != http.StatusOK {
		t.Fatalf("add viewer: %d %s", added.Status, added.Body)
	}
	if res := env.Do(t, testutil.Integration, http.MethodGet, detail, nil); res.Status != http.StatusOK {
		t.Fatalf("viewer reads ticket: %d %s", res.Status, res.Body)
	}
	if got := compatListedIDs(t, env, "/v1/tickets"); !contains(got, id) {
		t.Fatalf("viewer's ticket list = %v, want it to include %s", got, id)
	}

	// Public tickets.
	env.CallerFor = "assistant"
	publicID, _ := compatCreateTicket(t, env, testutil.Employee, "in_progress")
	testutil.AssertError(t, env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/"+publicID, nil), http.StatusForbidden, "invisible_ticket")
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=json_set(definition_json,'$.publicTicket',json('true')) WHERE id=?`, pf22ExpenseWorkflow); err != nil {
		t.Fatal(err)
	}
	res := env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/"+publicID, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("public ticket read: %d %s", res.Status, res.Body)
	}
	var summary struct {
		PublicStatus bool `json:"publicStatus"`
	}
	res.JSON(t, &summary)
	if !summary.PublicStatus {
		t.Fatalf("publicStatus=false for a ticket that is readable because it is public")
	}
	if got := compatListedIDs(t, env, "/v1/tickets"); !contains(got, publicID) {
		t.Fatalf("public ticket missing from list: %v", got)
	}
}

func compatListedIDs(t *testing.T, env *testutil.Env, path string) []string {
	t.Helper()
	res := env.Do(t, testutil.Integration, http.MethodGet, path, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("list %s: %d %s", path, res.Status, res.Body)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	res.JSON(t, &rows)
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
