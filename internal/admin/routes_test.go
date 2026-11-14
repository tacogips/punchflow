package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperationsContractAndUnknownRoute(t *testing.T) {
	ops := Operations()
	if len(ops) != 23 {
		t.Fatalf("Operations() has %d rows, want 23", len(ops))
	}
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		key := op.Method + " " + op.Pattern
		if seen[key] {
			t.Fatalf("duplicate operation route %q", key)
		}
		seen[key] = true
		if op.Name == "" || op.Handle == nil {
			t.Fatalf("incomplete operation row: %+v", op)
		}
	}

	mux := http.NewServeMux()
	New(Deps{}).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/api/unknown", nil))
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), `"code":"endpoint_not_found"`) {
		t.Fatalf("unknown admin route = %d %q, want JSON endpoint_not_found", rr.Code, rr.Body.String())
	}
}
