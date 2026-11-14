package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestProxyApplicantsCRUDAndValidation(t *testing.T) {
	env := testutil.New(t)
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/proxyApplicants?perPage=1", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	var page []map[string]any
	list.JSON(t, &page)
	if len(page) != 1 {
		t.Fatalf("list length=%d want 1", len(page))
	}
	if list.Header.Get("Page") != "1" || list.Header.Get("Per-Page") != "1" || list.Header.Get("Total") != "1" {
		t.Fatalf("pagination headers: %v", list.Header)
	}
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", map[string]any{"userId": "sales-rep", "proxyUserId": env.UserID(t, "assistant")}); got.Status != http.StatusBadRequest {
		t.Fatalf("business-code proxy IDs status=%d body=%s, want 400", got.Status, got.Body)
	}
	body := map[string]any{"userId": env.UserID(t, "sales-rep"), "proxyUserId": env.UserID(t, "assistant"), "workflowIds": []string{workflowID}}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", body)
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var value map[string]any
	created.JSON(t, &value)
	id := value["id"].(string)
	if got := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/proxyApplicants/"+id, map[string]any{"workflowIds": []string{"expense-claim"}}); got.Status != http.StatusBadRequest {
		t.Fatalf("workflow code in patch status=%d body=%s, want 400", got.Status, got.Body)
	}
	workflows, _ := value["workflows"].([]any)
	if len(workflows) != 1 {
		t.Fatalf("deduplicated workflows=%v", workflows)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", body); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate status=%d body=%s", got.Status, got.Body)
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/proxyApplicants/"+id, map[string]any{"endsOn": "2026-12-31"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Status, updated.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/proxyApplicants/"+id, nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("delete status=%d body=%s", deleted.Status, deleted.Body)
	}
	logs := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?sortBy=createdAt-desc", nil)
	if logs.Status != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", logs.Status, logs.Body)
	}
	var records []map[string]any
	logs.JSON(t, &records)
	actions := map[string]bool{}
	for _, x := range records {
		actions[x["action"].(string)] = true
	}
	if !actions["proxyApplicant.create"] || !actions["proxyApplicant.update"] || !actions["proxyApplicant.delete"] {
		t.Fatalf("proxy audit actions missing: %v", actions)
	}
}

func TestProxyApproversAndSelfServicePermission(t *testing.T) {
	env := testutil.New(t)
	res := env.Do(t, testutil.Admin, http.MethodGet, "/v1/proxyApprovers", nil)
	if res.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", res.Status, res.Body)
	}
	var rows []map[string]any
	res.JSON(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("approver count=%d want 1", len(rows))
	}
	body := map[string]any{"userId": env.UserID(t, "sales-rep"), "proxyUserId": env.UserID(t, "assistant")}
	if denied := env.Do(t, testutil.Employee, http.MethodPost, "/v1/proxyApprovers", body); denied.Status != http.StatusForbidden {
		t.Fatalf("self-service disabled status=%d body=%s", denied.Status, denied.Body)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), "UPDATE tenant SET self_service_proxy=1"); err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/proxyApprovers", body)
	if created.Status != http.StatusOK {
		t.Fatalf("self-service create status=%d body=%s", created.Status, created.Body)
	}
	var item map[string]any
	if err := json.Unmarshal(created.Body, &item); err != nil {
		t.Fatal(err)
	}
	id := item["id"].(string)
	if denied := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/proxyApprovers/"+id, map[string]any{"userId": env.UserID(t, "assistant")}); denied.Status != http.StatusForbidden {
		t.Fatalf("self-service owner change status=%d body=%s", denied.Status, denied.Body)
	}
	if deleted := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/proxyApprovers/"+id, nil); deleted.Status != http.StatusOK {
		t.Fatalf("self-service delete status=%d body=%s", deleted.Status, deleted.Body)
	}
}

func TestProxyApproverAPIAddsAndRetainsProcessingTicketSharing(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	var principalID string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT user_id FROM ticket_assignees WHERE ticket_id=? AND completed=0 ORDER BY created_at LIMIT 1`, ticketID).Scan(&principalID); err != nil {
		t.Fatal(err)
	}
	proxyID := env.UserID(t, "assistant")
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApprovers", map[string]any{"userId": principalID, "proxyUserId": proxyID})
	if created.Status != http.StatusOK {
		t.Fatalf("create proxy setting: %d %s", created.Status, created.Body)
	}
	var proxy struct {
		ID string `json:"id"`
	}
	created.JSON(t, &proxy)
	assertSharing := func(stage string) {
		t.Helper()
		viewers, err := store.TicketViewers(t.Context(), env.DB.Q(), ticketID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, viewer := range viewers {
			found = found || viewer.UserID != nil && *viewer.UserID == proxyID
		}
		if !found {
			t.Fatalf("proxy viewer missing %s setting", stage)
		}
		state, err := store.TicketWatchState(t.Context(), env.DB.Q(), ticketID, proxyID)
		if err != nil || state != store.WatchStateWatching {
			t.Fatalf("proxy watch state after %s = %q, err=%v", stage, state, err)
		}
	}
	assertSharing("creation")
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/proxyApprovers/"+proxy.ID, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("remove proxy setting: %d %s", deleted.Status, deleted.Body)
	}
	assertSharing("removal")
}
