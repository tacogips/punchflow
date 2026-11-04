package admin_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestTenantUpdateValidationAndMode(t *testing.T) {
	env := testutil.New(t)
	bad := env.Do(t, testutil.Admin, http.MethodPatch, "/admin/api/tenant", map[string]any{"ipAllowlist": []string{"not-a-cidr"}})
	testutil.AssertError(t, bad, http.StatusUnprocessableEntity, "validation_failed")
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/admin/api/tenant", map[string]any{"mode": "maintenance", "allowedEmailDomains": []string{"example.test"}})
	if updated.Status != http.StatusOK {
		t.Fatalf("update tenant: %d %s", updated.Status, updated.Body)
	}
	blocked := env.Do(t, testutil.Employee, http.MethodGet, "/v1/user", nil)
	testutil.AssertError(t, blocked, http.StatusForbidden, "tenant_maintenance")
}
