package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"

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
	body := map[string]any{"userId": env.UserID(t, "sales-rep"), "proxyUserId": env.UserID(t, "assistant"), "workflowIds": []string{"expense-claim", "EXPENSE-CLAIM"}}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", body)
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var value map[string]any
	created.JSON(t, &value)
	id := value["id"].(string)
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
