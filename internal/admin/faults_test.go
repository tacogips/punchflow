package admin_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestFaultCatalog(t *testing.T) {
	cases := []struct {
		code   string
		status int
	}{{"feature_disabled", 503}, {"gateway_timeout", 504}, {"email_not_verified", 401}, {"license_purchase_required", 403}, {"option_purchase_required", 403}, {"internal_server_error", 500}}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			env := testutil.New(t)
			created := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/faults", map[string]any{"pathPrefix": "/v1/tickets", "status": tc.status, "code": tc.code, "remaining": 1})
			if created.Status != http.StatusOK {
				t.Fatalf("create fault: %d %s", created.Status, created.Body)
			}
			first := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets", nil)
			testutil.AssertError(t, first, tc.status, tc.code)
			second := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets", nil)
			if second.Status == tc.status {
				t.Fatalf("one-shot fault repeated: %d %s", second.Status, second.Body)
			}
		})
	}
}
