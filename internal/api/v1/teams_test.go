package v1_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

const pf19Chart = "00000000-0000-4000-8000-000000000027"

func TestTeamTreeCreateUpdateAndPermissions(t *testing.T) {
	env := testutil.New(t)
	roots := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams?perPage=2", nil)
	if roots.Status != http.StatusOK {
		t.Fatalf("roots status=%d body=%s", roots.Status, roots.Body)
	}
	if roots.Header.Get("Page") != "1" || roots.Header.Get("Per-Page") != "2" || roots.Header.Get("Total") != "1" {
		t.Fatalf("pagination headers: %v", roots.Header)
	}
	children := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams?parentId=sales", nil)
	if children.Status != http.StatusOK {
		t.Fatalf("children status=%d body=%s", children.Status, children.Body)
	}
	var rows []map[string]any
	children.JSON(t, &rows)
	if len(rows) != 1 || rows[0]["code"] != "sales-east" {
		t.Fatalf("sales children=%v", rows)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+pf19Chart+"/teams", map[string]any{"name": "New Sales", "parentId": "company"})
	if created.Status != http.StatusOK {
		t.Fatalf("create team status=%d body=%s", created.Status, created.Body)
	}
	var team map[string]any
	created.JSON(t, &team)
	id := team["id"].(string)
	code := team["code"].(string)
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams/"+code, nil); got.Status != http.StatusOK {
		t.Fatalf("get by code status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/00000000-0000-4000-8000-000000000028/teams", map[string]any{"name": "Wrong parent", "parentId": "00000000-0000-4000-8000-000000000020"}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("cross-chart parent status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+pf19Chart+"/teams/company", map[string]any{"parentId": id}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("cycle status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams", nil); got.Status != http.StatusForbidden {
		t.Fatalf("employee status=%d want 403", got.Status)
	}
}
func TestTeamMembershipLeaderAndPaginationRules(t *testing.T) {
	env := testutil.New(t)
	base := "/v1/organizationCharts/" + pf19Chart + "/teams/finance/memberships"
	members := env.Do(t, testutil.Admin, http.MethodGet, base+"?perPage=1", nil)
	if members.Status != http.StatusOK {
		t.Fatalf("members status=%d body=%s", members.Status, members.Body)
	}
	testutil.AssertPagination(t, members, 1, 1, 2)
	var rows []map[string]any
	members.JSON(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("page length=%d", len(rows))
	}
	if _, ok := rows[0]["userType"]; ok {
		t.Fatalf("MemberUser must omit userType: %v", rows[0])
	}
	adminID := env.UserID(t, "admin")
	grades := []string{"00000000-0000-4000-8000-000000000006"}
	if got := env.Do(t, testutil.Admin, http.MethodPost, base, map[string]any{"users": []any{map[string]any{"id": adminID, "leader": false, "gradeIds": grades}}}); got.Status != http.StatusOK {
		t.Fatalf("add member status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodDelete, base, map[string]any{"userIds": []string{env.UserID(t, "finance-manager")}}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("remove only leader status=%d body=%s", got.Status, got.Body)
	}
	many := make([]any, 11)
	for i := range many {
		many[i] = map[string]any{"id": adminID, "leader": false, "gradeIds": grades}
	}
	tooMany := env.Do(t, testutil.Admin, http.MethodPost, base, map[string]any{"users": many})
	if tooMany.Status != http.StatusBadRequest {
		t.Fatalf("11 users status=%d body=%s", tooMany.Status, tooMany.Body)
	}
	var sizeLimitError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	tooMany.JSON(t, &sizeLimitError)
	if sizeLimitError.Code != "invalid_parameter" || !strings.Contains(sizeLimitError.Message, "between 1 and 10") {
		t.Fatalf("11 users error=%+v", sizeLimitError)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, base+"/accountant", map[string]any{"leader": true, "gradeIds": grades}); got.Status != http.StatusOK {
		t.Fatalf("update member status=%d body=%s", got.Status, got.Body)
	}
}
