package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestOrganizationChartListPermissionsAndCRUD(t *testing.T) {
	env := testutil.New(t)
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts?perPage=1", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	testutil.AssertPagination(t, list, 1, 1, 2)
	if denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/organizationCharts", nil); denied.Status != http.StatusForbidden {
		t.Fatalf("employee status=%d want 403", denied.Status)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{"name": "2028 Planning"})
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var chart map[string]any
	created.JSON(t, &chart)
	id := chart["id"].(string)
	if chart["current"] != false {
		t.Fatalf("new chart current=%v", chart["current"])
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+id, map[string]any{"name": "2028 Plan"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Status, updated.Body)
	}
	if current := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationChart", nil); current.Status != http.StatusOK {
		t.Fatalf("current status=%d body=%s", current.Status, current.Body)
	}
	active := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+id+"/activate", nil)
	if active.Status != http.StatusOK {
		t.Fatalf("activate status=%d body=%s", active.Status, active.Body)
	}
	active.JSON(t, &chart)
	if chart["current"] != true {
		t.Fatalf("activated current=%v", chart["current"])
	}
	if denied := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/organizationCharts/"+id, nil); denied.Status != http.StatusUnprocessableEntity {
		t.Fatalf("delete current status=%d want 422 body=%s", denied.Status, denied.Body)
	}
	old := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/organizationCharts/00000000-0000-4000-8000-000000000027", nil)
	if old.Status != http.StatusOK {
		t.Fatalf("delete old status=%d body=%s", old.Status, old.Body)
	}
	var n int
	if err := env.DB.Q().QueryRowContext(t.Context(), "SELECT count(*) FROM teams WHERE organization_chart_id=?", "00000000-0000-4000-8000-000000000027").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted chart retained %d teams", n)
	}
}
