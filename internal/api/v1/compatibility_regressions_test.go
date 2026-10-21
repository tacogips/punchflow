package v1_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestTicketListsFilterAuthorTeamFullName(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	fullName, err := store.TeamFullName(t.Context(), env.DB.Q(), teamID)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		persona testutil.Persona
		path    string
	}{
		{testutil.Admin, "/v1/tickets?authorTeamFullName=" + url.QueryEscape(fullName)},
		{testutil.Manager, "/v1/tickets/tasks?status=in_progress&authorTeamFullName=" + url.QueryEscape(fullName)},
	} {
		response := env.Do(t, request.persona, http.MethodGet, request.path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("GET %s: %d %s", request.path, response.Status, response.Body)
		}
		var tickets []struct {
			ID string `json:"id"`
		}
		response.JSON(t, &tickets)
		found := false
		for _, ticket := range tickets {
			found = found || ticket.ID == id
		}
		if !found {
			t.Fatalf("GET %s omitted ticket %s: %s", request.path, id, response.Body)
		}
		if response.Header.Get("Total") != "1" {
			t.Errorf("GET %s Total=%q, want 1", request.path, response.Header.Get("Total"))
		}
	}
}

func TestCurrentStepFiltersIgnoreFutureAssignments(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	var futureStep, futureTitle string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT id,title FROM ticket_steps WHERE ticket_id=? AND step_order=2`, id).Scan(&futureStep, &futureTitle); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_steps SET status='in_progress' WHERE id=?`, futureStep); err != nil {
		t.Fatal(err)
	}
	futureUser := env.UserID(t, "sales-rep")
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,assigned_at,created_at,updated_at) VALUES(?,?,?,?,0,1,1,1)`, store.NewID(), futureStep, id, futureUser); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/v1/tickets?stepTitle=" + url.QueryEscape(futureTitle),
		"/v1/tickets?assigneeUserId=" + futureUser + "&assigneeStatus=current",
	} {
		response := env.Do(t, testutil.Admin, http.MethodGet, path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Status, response.Body)
		}
		if response.Header.Get("Total") != "0" {
			t.Errorf("GET %s Total=%q, want 0", path, response.Header.Get("Total"))
		}
	}
	current := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?stepTitle="+url.QueryEscape("Manager approval"), nil)
	if current.Status != http.StatusOK || current.Header.Get("Total") != "1" {
		t.Fatalf("current step filter: %d Total=%q body=%s", current.Status, current.Header.Get("Total"), current.Body)
	}
}

func TestExplicitHistoricalTaskStatusKeepsCompletedAssignments(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET status='completed',current_step=0 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_steps SET status='completed',completed_at=2 WHERE ticket_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_assignees SET completed=1,completed_at=2 WHERE ticket_id=?`, id); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{{"/v1/tickets/tasks", "0"}, {"/v1/tickets/tasks?status=completed", "1"}} {
		response := env.Do(t, testutil.Manager, http.MethodGet, tc.path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("GET %s: %d %s", tc.path, response.Status, response.Body)
		}
		if got := response.Header.Get("Total"); got != tc.want {
			t.Errorf("GET %s Total=%q, want %s", tc.path, got, tc.want)
		}
	}
}

func TestGeneralMasterFieldRemovalCleansItemInputs(t *testing.T) {
	env := testutil.New(t)
	master := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{
		"code": "pf-api13", "name": "API13 master", "fields": []any{map[string]any{"code": "note", "title": "Note", "fieldType": "text"}},
	})
	if master.Status != http.StatusOK {
		t.Fatalf("create master: %d %s", master.Status, master.Body)
	}
	item := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/pf-api13/items", map[string]any{"code": "entry", "name": "Entry", "inputs": []any{map[string]any{"code": "note", "value": "keep?"}}})
	if item.Status != http.StatusOK {
		t.Fatalf("create item: %d %s", item.Status, item.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	item.JSON(t, &created)
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api13", map[string]any{"fields": []any{}})
	if updated.Status != http.StatusOK {
		t.Fatalf("remove field: %d %s", updated.Status, updated.Body)
	}
	for _, path := range []string{"/v1/generalMasters/pf-api13/items/" + created.ID, "/v1/generalMasters/pf-api13/items"} {
		response := env.Do(t, testutil.Admin, http.MethodGet, path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("GET %s after field removal: %d %s", path, response.Status, response.Body)
		}
		if strings.Contains(string(response.Body), "keep?") {
			t.Errorf("removed field value leaked in %s: %s", path, response.Body)
		}
	}
}

func TestAdminResourceMutationLengthsMatchOpenAPILimits(t *testing.T) {
	env := testutil.New(t)
	long := func(n int) string { return strings.Repeat("x", n) }
	assertValidation := func(label string, response *testutil.Response) {
		t.Helper()
		if response.Status != http.StatusUnprocessableEntity {
			t.Errorf("%s status=%d body=%s, want validation_failed", label, response.Status, response.Body)
		}
	}
	master := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"code": "pf-api12", "name": "API12", "fields": []any{map[string]any{"code": "field", "title": "Field", "fieldType": "text"}}})
	if master.Status != http.StatusOK {
		t.Fatalf("create master: %d %s", master.Status, master.Body)
	}
	assertValidation("create master name", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": long(301)}))
	assertValidation("create master description", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Master", "description": long(10001)}))
	assertValidation("create master field title", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Master", "fields": []any{map[string]any{"code": "f", "title": long(301), "fieldType": "text"}}}))
	assertValidation("create master field code", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Master", "fields": []any{map[string]any{"code": long(101), "title": "F", "fieldType": "text"}}}))
	assertValidation("create master field description", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Master", "fields": []any{map[string]any{"code": "f", "title": "F", "description": long(10001), "fieldType": "text"}}}))
	assertValidation("update master name", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"name": long(301)}))
	assertValidation("update master description", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"description": long(10001)}))
	assertValidation("update master code", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"code": long(101)}))
	assertValidation("update master field description", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"fields": []any{map[string]any{"code": "field", "description": long(10001)}}}))
	assertValidation("master field title", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"fields": []any{map[string]any{"code": "f", "title": long(301), "fieldType": "text"}}}))
	assertValidation("master field code", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12", map[string]any{"fields": []any{map[string]any{"code": long(101), "title": "F", "fieldType": "text"}}}))
	assertValidation("create item name", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/pf-api12/items", map[string]any{"name": long(101), "inputs": []any{}}))
	assertValidation("create item code", env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/pf-api12/items", map[string]any{"code": long(101), "name": "item", "inputs": []any{}}))
	validItem := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/pf-api12/items", map[string]any{"code": "valid", "name": "item", "inputs": []any{}})
	if validItem.Status != http.StatusOK {
		t.Fatalf("create valid item: %d %s", validItem.Status, validItem.Body)
	}
	var item struct {
		ID string `json:"id"`
	}
	validItem.JSON(t, &item)
	assertValidation("update item name", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12/items/"+item.ID, map[string]any{"name": long(101)}))
	assertValidation("update item code", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/pf-api12/items/"+item.ID, map[string]any{"code": long(101)}))
	chart, err := store.ActiveOrgChartID(t.Context(), env.DB.Q())
	if err != nil {
		t.Fatal(err)
	}
	assertValidation("create team name", env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+chart+"/teams", map[string]any{"name": long(301)}))
	assertValidation("create team code", env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+chart+"/teams", map[string]any{"name": "team", "code": long(101)}))
	assertValidation("create team notes", env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+chart+"/teams", map[string]any{"name": "team", "notes": long(10001)}))
	validTeam := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+chart+"/teams", map[string]any{"name": "Valid team", "code": "valid-team"})
	if validTeam.Status != http.StatusOK {
		t.Fatalf("create valid team: %d %s", validTeam.Status, validTeam.Body)
	}
	var team struct {
		ID string `json:"id"`
	}
	validTeam.JSON(t, &team)
	assertValidation("update team name", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart+"/teams/"+team.ID, map[string]any{"name": long(301)}))
	assertValidation("update team code", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart+"/teams/"+team.ID, map[string]any{"code": long(101)}))
	assertValidation("update team notes", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart+"/teams/"+team.ID, map[string]any{"notes": long(10001)}))
	assertValidation("create role name", env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{"name": long(301), "permissionList": []any{}}))
	validRole := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{"name": "Valid role", "permissionList": []any{}})
	if validRole.Status != http.StatusOK {
		t.Fatalf("create valid role: %d %s", validRole.Status, validRole.Body)
	}
	var role struct {
		ID string `json:"id"`
	}
	validRole.JSON(t, &role)
	assertValidation("update role name", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/roles/"+role.ID, map[string]any{"name": long(301)}))
}
