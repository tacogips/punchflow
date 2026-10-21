package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

const (
	scopeFolderFinance   = "00000000-0000-4000-8000-000000000037"
	scopeFolderGeneral   = "00000000-0000-4000-8000-000000000038"
	scopeMasterCostCtr   = "00000000-0000-4000-8000-000000000039"
	scopeMasterVendors   = "00000000-0000-4000-8000-000000000047"
	scopeTeamSales       = "00000000-0000-4000-8000-000000000021"
	scopeTeamSalesEast   = "00000000-0000-4000-8000-000000000022"
	scopeTeamEngineering = "00000000-0000-4000-8000-000000000023"
	scopeChart2026       = "00000000-0000-4000-8000-000000000027"
)

// grantRole creates a role with the permission list and makes the fixture user
// (by code) a member, using the unrestricted admin persona.
func grantRole(t *testing.T, env *testutil.Env, userCode string, permissions ...map[string]any) {
	t.Helper()
	list := make([]any, len(permissions))
	for i, p := range permissions {
		list[i] = p
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{"name": "Scoped " + userCode, "permissionList": list})
	if created.Status != http.StatusOK {
		t.Fatalf("create role: status=%d body=%s", created.Status, created.Body)
	}
	var role struct {
		ID string `json:"id"`
	}
	created.JSON(t, &role)
	member := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+role.ID+"/memberships", map[string]any{"userIds": []string{env.UserID(t, userCode)}})
	if member.Status != http.StatusOK {
		t.Fatalf("add role member: status=%d body=%s", member.Status, member.Body)
	}
}

func restricted(permission string, folders, masters, teams []string) map[string]any {
	nonNil := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}
	return map[string]any{"permission": permission, "restricted": true, "folderIds": nonNil(folders), "generalMasterIds": nonNil(masters), "teamIds": nonNil(teams)}
}

func ids(t *testing.T, res *testutil.Response) []string {
	t.Helper()
	var rows []map[string]any
	res.JSON(t, &rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row["id"].(string))
	}
	return out
}

func TestRestrictedMasterRoleOnlyCoversListedMasters(t *testing.T) {
	env := testutil.New(t)
	// Baseline: with no role the employee cannot reach any master.
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters", nil); got.Status != http.StatusForbidden {
		t.Fatalf("baseline list status=%d want 403", got.Status)
	}
	grantRole(t, env, "sales-rep", restricted("master", nil, []string{scopeMasterCostCtr}, nil))

	list := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("restricted list status=%d body=%s", list.Status, list.Body)
	}
	if got := ids(t, list); len(got) != 1 || got[0] != scopeMasterCostCtr {
		t.Fatalf("restricted list = %v, want only the cost-centers master", got)
	}
	if list.Header.Get("Total") != "1" {
		t.Fatalf("Total header = %q, want 1", list.Header.Get("Total"))
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters", nil); len(ids(t, got)) != 2 {
		t.Fatalf("admin must still see both masters: %s", got.Body)
	}

	for _, tc := range []struct {
		name, method, path string
		body               any
		want               int
	}{
		{"get in scope", http.MethodGet, "/v1/generalMasters/" + scopeMasterCostCtr, nil, http.StatusOK},
		{"get out of scope", http.MethodGet, "/v1/generalMasters/" + scopeMasterVendors, nil, http.StatusForbidden},
		{"patch in scope", http.MethodPatch, "/v1/generalMasters/" + scopeMasterCostCtr, map[string]any{"description": "scoped edit"}, http.StatusOK},
		{"patch out of scope", http.MethodPatch, "/v1/generalMasters/" + scopeMasterVendors, map[string]any{"description": "nope"}, http.StatusForbidden},
		{"delete out of scope", http.MethodDelete, "/v1/generalMasters/" + scopeMasterVendors, nil, http.StatusForbidden},
		{"items out of scope", http.MethodGet, "/v1/generalMasters/" + scopeMasterVendors + "/items", nil, http.StatusForbidden},
		{"items in scope", http.MethodGet, "/v1/generalMasters/" + scopeMasterCostCtr + "/items", nil, http.StatusOK},
		{"create item out of scope", http.MethodPost, "/v1/generalMasters/" + scopeMasterVendors + "/items", map[string]any{"name": "x", "inputs": []any{}}, http.StatusForbidden},
		{"create master needs unrestricted", http.MethodPost, "/v1/generalMasters", map[string]any{"name": "New master"}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := env.Do(t, testutil.Employee, tc.method, tc.path, tc.body)
			if got.Status != tc.want {
				t.Fatalf("%s %s status=%d want %d body=%s", tc.method, tc.path, got.Status, tc.want, got.Body)
			}
			if tc.want == http.StatusForbidden {
				testutil.AssertError(t, got, http.StatusForbidden, "missing_permission")
			}
		})
	}
}

func TestRestrictedMasterReadDoesNotAllowWrites(t *testing.T) {
	env := testutil.New(t)
	grantRole(t, env, "sales-rep", restricted("master_read", nil, []string{scopeMasterVendors}, nil))
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters/"+scopeMasterVendors, nil); got.Status != http.StatusOK {
		t.Fatalf("read in scope status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters/"+scopeMasterCostCtr, nil); got.Status != http.StatusForbidden {
		t.Fatalf("read out of scope status=%d want 403", got.Status)
	}
	if got := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/generalMasters/"+scopeMasterVendors, map[string]any{"description": "x"}); got.Status != http.StatusForbidden {
		t.Fatalf("write with read-only scope status=%d want 403", got.Status)
	}
}

func TestUnrestrictedMasterRoleStaysGlobal(t *testing.T) {
	env := testutil.New(t)
	grantRole(t, env, "sales-rep", map[string]any{"permission": "master", "restricted": false})
	if got := ids(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters", nil)); len(got) != 2 {
		t.Fatalf("unrestricted master list = %v, want both masters", got)
	}
	if got := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/generalMasters/"+scopeMasterVendors, map[string]any{"description": "ok"}); got.Status != http.StatusOK {
		t.Fatalf("unrestricted patch status=%d body=%s", got.Status, got.Body)
	}
}

func TestRestrictedTeamRoleCoversTeamSubtree(t *testing.T) {
	env := testutil.New(t)
	grantRole(t, env, "sales-rep", restricted("team", nil, nil, []string{scopeTeamSales}))
	base := "/v1/organizationCharts/" + scopeChart2026 + "/teams"

	// A restricted team administrator sees only the covered subtree.
	children := env.Do(t, testutil.Employee, http.MethodGet, base+"?parentId=company", nil)
	if children.Status != http.StatusOK {
		t.Fatalf("list children status=%d body=%s", children.Status, children.Body)
	}
	if got := ids(t, children); len(got) != 1 || got[0] != scopeTeamSales {
		t.Fatalf("children of company = %v, want only sales", got)
	}
	if got := ids(t, env.Do(t, testutil.Employee, http.MethodGet, base, nil)); len(got) != 0 {
		t.Fatalf("root team list = %v, want none (company is outside the scope)", got)
	}

	for _, tc := range []struct {
		name, method, path string
		body               any
		want               int
	}{
		{"get covered team", http.MethodGet, base + "/" + scopeTeamSales, nil, http.StatusOK},
		{"get descendant", http.MethodGet, base + "/" + scopeTeamSalesEast, nil, http.StatusOK},
		{"get other team", http.MethodGet, base + "/" + scopeTeamEngineering, nil, http.StatusForbidden},
		{"update descendant", http.MethodPatch, base + "/" + scopeTeamSalesEast, map[string]any{"notes": "scoped"}, http.StatusOK},
		{"update other team", http.MethodPatch, base + "/" + scopeTeamEngineering, map[string]any{"notes": "nope"}, http.StatusForbidden},
		{"move out of scope", http.MethodPatch, base + "/" + scopeTeamSalesEast, map[string]any{"parentId": scopeTeamEngineering}, http.StatusForbidden},
		{"create under covered team", http.MethodPost, base, map[string]any{"name": "Sales west", "parentId": scopeTeamSales}, http.StatusOK},
		{"create under other team", http.MethodPost, base, map[string]any{"name": "Rogue", "parentId": scopeTeamEngineering}, http.StatusForbidden},
		{"create root team", http.MethodPost, base, map[string]any{"name": "Rogue root"}, http.StatusForbidden},
		{"members of other team", http.MethodGet, base + "/" + scopeTeamEngineering + "/memberships", nil, http.StatusForbidden},
		{"members of covered team", http.MethodGet, base + "/" + scopeTeamSales + "/memberships", nil, http.StatusOK},
		{"delete other team", http.MethodDelete, base + "/" + scopeTeamEngineering, nil, http.StatusForbidden},
		{"chart create needs unrestricted", http.MethodPost, "/v1/organizationCharts", map[string]any{"name": "New chart"}, http.StatusForbidden},
		{"chart update needs unrestricted", http.MethodPatch, "/v1/organizationCharts/" + scopeChart2026, map[string]any{"name": "Renamed"}, http.StatusForbidden},
		{"chart activate needs unrestricted", http.MethodPost, "/v1/organizationCharts/" + scopeChart2026 + "/activate", nil, http.StatusForbidden},
		{"chart read allowed", http.MethodGet, "/v1/organizationCharts/" + scopeChart2026, nil, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := env.Do(t, testutil.Employee, tc.method, tc.path, tc.body)
			if got.Status != tc.want {
				t.Fatalf("%s %s status=%d want %d body=%s", tc.method, tc.path, got.Status, tc.want, got.Body)
			}
		})
	}
}

func TestRestrictedTicketReadCoversOnlyListedFolders(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	env.CallerFor = "engineer"

	if got := env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/"+ticketID, nil); got.Status != http.StatusForbidden {
		t.Fatalf("baseline ticket read status=%d want 403 body=%s", got.Status, got.Body)
	}
	// A ticket_read grant restricted to the unrelated general folder still hides the finance ticket.
	grantRole(t, env, "engineer", restricted("ticket_read", []string{scopeFolderGeneral}, nil, nil))
	if got := env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/"+ticketID, nil); got.Status != http.StatusForbidden {
		t.Fatalf("out-of-scope ticket read status=%d want 403 body=%s", got.Status, got.Body)
	}
	if got := ids(t, env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets", nil)); len(got) != 0 {
		t.Fatalf("out-of-scope ticket list = %v, want none", got)
	}
	// Restricting to the workflow's folder (or one of its ancestors) grants access.
	grantRole(t, env, "engineer", restricted("ticket_read", []string{scopeFolderFinance}, nil, nil))
	if got := env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets/"+ticketID, nil); got.Status != http.StatusOK {
		t.Fatalf("in-scope ticket read status=%d body=%s", got.Status, got.Body)
	}
	if got := ids(t, env.Do(t, testutil.Integration, http.MethodGet, "/v1/tickets", nil)); len(got) != 1 || got[0] != ticketID {
		t.Fatalf("in-scope ticket list = %v, want [%s]", got, ticketID)
	}
}
