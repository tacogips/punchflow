package v1_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestAuditLogFiltersAndOrdering(t *testing.T) {
	env := testutil.New(t)
	adminID := env.UserID(t, "admin")
	otherID := env.UserID(t, "sales-rep")
	base := env.Clock.Now()
	for i, actor := range []string{adminID, otherID, adminID} {
		actorID := actor
		if err := store.InsertAuditLog(t.Context(), env.DB.Q(), model.AuditLog{
			UserID: &actorID, Action: "test.audit", CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	res := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?perPage=2", nil)
	if res.Status != http.StatusOK {
		t.Fatalf("audit list status = %d, body=%s", res.Status, res.Body)
	}
	testutil.AssertPagination(t, res, 1, 2, 3+pf21ExpectedSeedAuditLogs(t, env))
	var items []map[string]any
	res.JSON(t, &items)
	if len(items) != 2 {
		t.Fatalf("audit page length = %d, want 2", len(items))
	}
	if items[0]["createdAt"].(string) < items[1]["createdAt"].(string) {
		t.Fatalf("default audit order is not descending: %v then %v", items[0]["createdAt"], items[1]["createdAt"])
	}
	filtered := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?userId="+url.QueryEscape(otherID), nil)
	if filtered.Status != http.StatusOK {
		t.Fatalf("userId filter status = %d, body=%s", filtered.Status, filtered.Body)
	}
	filtered.JSON(t, &items)
	for _, item := range items {
		user := item["user"].(map[string]any)
		if user["id"] != otherID {
			t.Fatalf("userId filter returned actor %v, want %s", user["id"], otherID)
		}
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?createdAtStart=not-a-time", nil); got.Status != http.StatusBadRequest {
		t.Fatalf("bad timestamp status = %d, want 400", got.Status)
	}
	endOnly := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?createdAtEnd=2025-12-31", nil)
	if endOnly.Status != http.StatusOK {
		t.Fatalf("createdAtEnd filter status = %d, body=%s", endOnly.Status, endOnly.Body)
	}
	endOnly.JSON(t, &items)
	if len(items) != 0 {
		t.Fatalf("createdAtEnd filter returned %d logs, want 0", len(items))
	}
	start := url.QueryEscape("2026-01-02")
	end := url.QueryEscape("2026-01-01")
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?createdAtStart="+start+"&createdAtEnd="+end, nil); got.Status != http.StatusBadRequest {
		t.Fatalf("reversed timestamp range status = %d, want 400", got.Status)
	}
}

func pf21ExpectedSeedAuditLogs(t *testing.T, env *testutil.Env) int {
	t.Helper()
	var count int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action!='test.audit'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
