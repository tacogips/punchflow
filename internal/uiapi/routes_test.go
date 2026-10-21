package uiapi_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/testutil"
	"punchflow/internal/uiapi"
)

func TestRouteTableStaysOutOfV1(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range uiapi.Operations() {
		if !strings.HasPrefix(op.Pattern, "/ui/api/") {
			t.Errorf("%s pattern %q is outside /ui/api/", op.Name, op.Pattern)
		}
		key := op.Method + " " + op.Pattern
		if seen[key] {
			t.Errorf("duplicate route %s", key)
		}
		seen[key] = true
	}
}

func TestUnknownUIEndpoint(t *testing.T) {
	e := newExpenseEnv(t)
	testutil.AssertError(t, e.Do(t, testutil.Employee, http.MethodGet, "/ui/api/nope", nil), http.StatusNotFound, "endpoint_not_found")
}

func TestWriteEndpointsRequireCSRFForBrowserSessions(t *testing.T) {
	// Watch and read-state changes use POST/DELETE so the shared authentication
	// middleware applies its CSRF check to session cookies; token callers are
	// exempt. This pins the method choice rather than the middleware itself.
	for _, op := range uiapi.Operations() {
		switch op.Name {
		case "watchTicket", "markNotificationRead", "markAllNotificationsRead", "setAssigneePending":
			if op.Method != http.MethodPost {
				t.Errorf("%s uses %s, want POST", op.Name, op.Method)
			}
		case "unwatchTicket":
			if op.Method != http.MethodDelete {
				t.Errorf("%s uses %s, want DELETE", op.Name, op.Method)
			}
		default:
			if op.Method != http.MethodGet {
				t.Errorf("%s uses %s; only the state-changing operations may not be GET", op.Name, op.Method)
			}
		}
	}
}
