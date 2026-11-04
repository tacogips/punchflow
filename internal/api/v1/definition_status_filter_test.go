package v1_test

import (
	"net/http"
	"sort"
	"testing"

	"punchflow/internal/testutil"
)

func codesOf(t *testing.T, res *testutil.Response) []string {
	t.Helper()
	if res.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var rows []map[string]any
	res.JSON(t, &rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row["code"].(string))
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestWorkflowListStatusArrayFilter(t *testing.T) {
	env := testutil.New(t)
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET status='invisible' WHERE code='leave-request'`); err != nil {
		t.Fatal(err)
	}
	all := []string{"expense-claim", "leave-request", "purchase-request"}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", all},
		{"?status[]=invisible", []string{"leave-request"}},
		{"?status[]=visible", []string{"expense-claim", "purchase-request"}},
		{"?status[]=visible&status[]=invisible", all},
		{"?status=invisible", []string{"leave-request"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got := codesOf(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows"+tc.query, nil))
			if !equalStrings(got, tc.want) {
				t.Fatalf("GET /v1/workflows%s = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
	if res := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows?status[]=visible&status[]=bogus", nil); res.Status != http.StatusBadRequest {
		t.Fatalf("invalid array member status=%d want 400 body=%s", res.Status, res.Body)
	}
	if res := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows?status[]=error", nil); res.Status != http.StatusBadRequest {
		t.Fatalf("route-only status on workflows status=%d want 400", res.Status)
	}
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows?status=invisible&status=visible", nil), http.StatusBadRequest, "invalid_parameter")
}

func TestRouteListStatusArrayFilter(t *testing.T) {
	env := testutil.New(t)
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE routes SET status='error' WHERE code='purchase'`); err != nil {
		t.Fatal(err)
	}
	all := []string{"expense-large", "expense-standard", "leave", "purchase"}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", all},
		{"?status[]=error", []string{"purchase"}},
		{"?status[]=visible", []string{"expense-large", "expense-standard", "leave"}},
		{"?status[]=visible&status[]=error", all},
		{"?status[]=error&status[]=visible", all},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got := codesOf(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes"+tc.query, nil))
			if !equalStrings(got, tc.want) {
				t.Fatalf("GET /v1/routes%s = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
	if res := env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes?status[]=invisible", nil); res.Status != http.StatusBadRequest {
		t.Fatalf("workflow-only status on routes status=%d want 400", res.Status)
	}
}
