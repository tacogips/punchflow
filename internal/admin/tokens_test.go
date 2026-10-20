package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestTokensCreateListDelete(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Employee, http.MethodPost, "/admin/api/tokens", map[string]any{"name": "client-test"})
	if created.Status != http.StatusOK {
		t.Fatalf("create token: %d %s", created.Status, created.Body)
	}
	var token map[string]any
	if err := json.Unmarshal(created.Body, &token); err != nil {
		t.Fatal(err)
	}
	plain, _ := token["token"].(string)
	id, _ := token["id"].(string)
	if plain == "" || id == "" {
		t.Fatalf("token response missing one-time token/id: %s", created.Body)
	}
	listed := env.Do(t, testutil.Employee, http.MethodGet, "/admin/api/tokens", nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("list: %d %s", listed.Status, listed.Body)
	}
	var rows []map[string]any
	listed.JSON(t, &rows)
	found := false
	for _, row := range rows {
		if row["id"] == id {
			found = true
			if _, ok := row["token"]; ok {
				t.Fatal("list returned plaintext token")
			}
		}
	}
	if !found {
		t.Fatalf("created token missing from list: %s", listed.Body)
	}
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/v1/user", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	if got := env.DoRaw(t, req); got.Status == http.StatusUnauthorized {
		t.Fatalf("issued PAT was rejected: %s", got.Body)
	}
	deleted := env.Do(t, testutil.Employee, http.MethodDelete, "/admin/api/tokens/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete: %d %s", deleted.Status, deleted.Body)
	}
	if got := env.DoRaw(t, req); got.Status != http.StatusUnauthorized {
		t.Fatalf("revoked token status=%d, want 401", got.Status)
	}
}
func TestServiceAccountPermissionAndCaller(t *testing.T) {
	env := testutil.New(t)
	if got := env.Do(t, testutil.Employee, http.MethodPost, "/admin/api/service-accounts", map[string]any{"name": "x"}); got.Status != http.StatusForbidden {
		t.Fatalf("employee create SAT status=%d", got.Status)
	}
	created := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/service-accounts", map[string]any{"name": "integration test", "description": "test"})
	if created.Status != http.StatusOK {
		t.Fatalf("create SAT: %d %s", created.Status, created.Body)
	}
	var body map[string]any
	created.JSON(t, &body)
	plain, _ := body["token"].(string)
	if plain == "" {
		t.Fatalf("SAT token absent: %s", created.Body)
	}
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/v1/user", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	got := env.DoRaw(t, req)
	if got.Status != http.StatusUnauthorized {
		t.Fatalf("SAT without caller status=%d", got.Status)
	}
	req.Header.Set("X-Caller-Id", env.UserID(t, "admin"))
	got = env.DoRaw(t, req)
	if got.Status == http.StatusUnauthorized {
		t.Fatalf("SAT with caller rejected: %s", got.Body)
	}
}
