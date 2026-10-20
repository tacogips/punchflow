package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestWorkflowRouteReadAPI(t *testing.T) {
	env := testutil.New(t)
	workflows := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows?perPage=2", nil)
	if workflows.Status != http.StatusOK {
		t.Fatalf("workflows=%d %s", workflows.Status, workflows.Body)
	}
	var ws []map[string]any
	workflows.JSON(t, &ws)
	if len(ws) != 2 {
		t.Fatalf("workflow page len=%d", len(ws))
	}
	testutil.AssertPagination(t, workflows, 1, 2, 3)
	wf := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if wf.Status != http.StatusOK {
		t.Fatalf("workflow detail=%d %s", wf.Status, wf.Body)
	}
	var wd map[string]any
	wf.JSON(t, &wd)
	for _, key := range []string{"sectionList", "routeConditions", "slackChannel"} {
		if _, ok := wd[key]; !ok {
			t.Errorf("workflow missing %s", key)
		}
	}
	if bad := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows?status=error", nil); bad.Status != http.StatusBadRequest {
		t.Fatalf("workflow invalid status=%d %s", bad.Status, bad.Body)
	}
	routes := env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes?perPage=2", nil)
	if routes.Status != http.StatusOK {
		t.Fatalf("routes=%d %s", routes.Status, routes.Body)
	}
	filtered := env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes?folderId=finance", nil)
	if filtered.Status != http.StatusOK {
		t.Fatalf("folder-filtered routes=%d %s", filtered.Status, filtered.Body)
	}
	var financeRoutes []map[string]any
	filtered.JSON(t, &financeRoutes)
	for _, row := range financeRoutes {
		folder := row["folder"].(map[string]any)
		if folder["code"] != "finance" {
			t.Fatalf("folder filter returned route in %v", folder["code"])
		}
	}
	route := env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes/expense-large", nil)
	if route.Status != http.StatusOK {
		t.Fatalf("route detail=%d %s", route.Status, route.Body)
	}
	var rd map[string]any
	route.JSON(t, &rd)
	steps, ok := rd["steps"].([]any)
	if !ok || len(steps) != 4 {
		t.Fatalf("route steps=%v", rd["steps"])
	}
	if bad := env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes?status=invisible", nil); bad.Status != http.StatusBadRequest {
		t.Fatalf("route invalid status=%d %s", bad.Status, bad.Body)
	}
}
