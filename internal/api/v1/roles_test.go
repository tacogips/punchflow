package v1_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

func TestRolesListPermissionsAndPagination(t *testing.T) {
	env := testutil.New(t)
	res := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles?perPage=2", nil)
	if res.Status != http.StatusOK {
		t.Fatalf("list roles status = %d, body=%s", res.Status, res.Body)
	}
	var roles []map[string]any
	res.JSON(t, &roles)
	if len(roles) != 2 {
		t.Fatalf("roles page length = %d, want 2", len(roles))
	}
	testutil.AssertPagination(t, res, 1, 2, 4)
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/roles", nil); got.Status != http.StatusForbidden {
		t.Fatalf("employee list status = %d, want 403", got.Status)
	}
}

func TestRoleCreateValidatesPermissionsAndTargets(t *testing.T) {
	env := testutil.New(t)
	folderID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO folders(id,code,name,editable,created_at,updated_at) VALUES(?,?,?,?,?,?)`, folderID, "case-target", "Case target", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	create := func(permission string, folderIDs []string) *testutil.Response {
		return env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{
			"name": "Auditor", "permissionList": []any{map[string]any{
				"permission": permission, "restricted": true, "folderIds": folderIDs,
				"generalMasterIds": []string{}, "teamIds": []string{},
			}},
		})
	}
	if got := create("audit_read", nil); got.Status != http.StatusBadRequest {
		t.Fatalf("audit_read status = %d, want 400 (body %s)", got.Status, got.Body)
	}
	if got := create("workflow", []string{folderID, folderID}); got.Status != http.StatusBadRequest {
		t.Fatalf("exact duplicate status = %d, want 400 (body %s)", got.Status, got.Body)
	}
	if got := create("workflow", []string{folderID, strings.ToUpper(folderID)}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("case-only duplicate status = %d, want 422 (body %s)", got.Status, got.Body)
	}
	if got := create("workflow", []string{"00000000-0000-4000-8000-000000000001"}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown target status = %d, want 422 (body %s)", got.Status, got.Body)
	}
	got := create("workflow", []string{folderID})
	if got.Status != http.StatusOK {
		t.Fatalf("valid role status = %d, body=%s", got.Status, got.Body)
	}
	var role struct {
		ID             string `json:"id"`
		PermissionList []struct {
			Permission string           `json:"permission"`
			Restricted bool             `json:"restricted"`
			Folders    []map[string]any `json:"folders"`
		} `json:"permissionList"`
	}
	got.JSON(t, &role)
	if role.ID == "" || len(role.PermissionList) != 1 || role.PermissionList[0].Permission != "workflow" || !role.PermissionList[0].Restricted || len(role.PermissionList[0].Folders) != 1 {
		t.Fatalf("role detail did not resolve target folder: %+v", role)
	}
}

func TestRoleMembershipAndSelfEditGuards(t *testing.T) {
	env := testutil.New(t)
	var superRole string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT id FROM roles WHERE editable=0`).Scan(&superRole); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		res := env.Do(t, testutil.Admin, method, "/v1/roles/"+superRole, map[string]any{"name": "Renamed"})
		if res.Status != http.StatusForbidden {
			t.Fatalf("super admin role %s status = %d, want 403 (body %s)", method, res.Status, res.Body)
		}
		var body map[string]any
		res.JSON(t, &body)
		if body["code"] != "resource_not_editable" {
			t.Fatalf("super admin role %s error code = %v, want resource_not_editable", method, body["code"])
		}
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+superRole, nil); got.Status != http.StatusOK {
		t.Fatalf("super admin role after denied mutations status = %d, want 200 (body %s)", got.Status, got.Body)
	}
	adminID := env.UserID(t, "admin")
	if got := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+superRole+"/memberships/"+adminID, nil); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("last super admin removal status = %d, want 422 (body %s)", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+superRole+"/memberships", map[string]any{"userIds": strings.Fields(strings.Repeat(adminID+" ", 11))}); got.Status != http.StatusBadRequest {
		t.Fatalf("11-member request status = %d, want 400 (body %s)", got.Status, got.Body)
	}
	create := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{
		"name": "Self role", "permissionList": []any{map[string]any{"permission": "role", "restricted": false}},
	})
	if create.Status != http.StatusOK {
		t.Fatalf("create role status = %d, body=%s", create.Status, create.Body)
	}
	var created map[string]any
	create.JSON(t, &created)
	roleID := created["id"].(string)
	engManagerID := env.UserID(t, "eng-manager")
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+roleID+"/memberships", map[string]any{"userIds": []string{engManagerID}}); got.Status != http.StatusOK {
		t.Fatalf("add eng-manager status = %d, body=%s", got.Status, got.Body)
	}
	env.CallerFor = "eng-manager"
	if got := env.Do(t, testutil.Integration, http.MethodPatch, "/v1/roles/"+roleID, map[string]any{"name": "Changed"}); got.Status != http.StatusForbidden {
		t.Fatalf("member own-role update status = %d, want 403 (body %s)", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Integration, http.MethodPost, "/v1/roles/"+roleID+"/memberships", map[string]any{"userIds": []string{engManagerID}}); got.Status != http.StatusForbidden {
		t.Fatalf("member self-add status = %d, want 403 (body %s)", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID, nil); got.Status != http.StatusOK {
		t.Fatalf("get role status = %d, body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID+"/memberships", nil); got.Status != http.StatusOK {
		t.Fatalf("list role members status = %d, body=%s", got.Status, got.Body)
	} else {
		var members []map[string]any
		got.JSON(t, &members)
		if len(members) != 1 || members[0]["id"] != engManagerID {
			t.Fatalf("role members = %v, want only eng-manager", members)
		}
		if _, hasCustomFields := members[0]["customFields"]; hasCustomFields {
			t.Fatalf("role member unexpectedly contains customFields: %v", members[0])
		}
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/roles/"+roleID, map[string]any{"name": "Renamed self role"}); got.Status != http.StatusOK {
		t.Fatalf("admin update role status = %d, body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+roleID+"/memberships/"+engManagerID, nil); got.Status != http.StatusOK || len(got.Body) != 0 {
		t.Fatalf("delete role member status/body = %d/%q, want 200/empty", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+roleID+"/memberships", map[string]any{"userIds": []string{engManagerID}}); got.Status != http.StatusOK {
		t.Fatalf("re-add role member status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+roleID+"/memberships/eng-manager", nil); got.Status != http.StatusOK || len(got.Body) != 0 {
		t.Fatalf("delete role member by user code status/body = %d/%q, want 200/empty", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+roleID, nil); got.Status != http.StatusOK || len(got.Body) != 0 {
		t.Fatalf("delete role status/body = %d/%q, want 200/empty", got.Status, got.Body)
	}
}

func TestAuditLogRoleMutationsAndFilters(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{
		"name": "Audit reader", "permissionList": []any{map[string]any{"permission": "audit", "restricted": false}},
	})
	if created.Status != http.StatusOK {
		t.Fatalf("create role status = %d, body=%s", created.Status, created.Body)
	}
	var role struct {
		ID string `json:"id"`
	}
	created.JSON(t, &role)
	if role.ID == "" {
		t.Fatal("created audit role has an empty id")
	}
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/auditLogs", nil); got.Status != http.StatusForbidden {
		t.Fatalf("employee audit status before role membership = %d, want 403 (body %s)", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+role.ID+"/memberships", map[string]any{"userIds": []string{env.UserID(t, "sales-rep")}}); got.Status != http.StatusOK {
		t.Fatalf("add sales-rep to audit role status = %d, body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/auditLogs", nil); got.Status != http.StatusOK {
		t.Fatalf("employee audit status after role membership = %d, want 200 (body %s)", got.Status, got.Body)
	}
	res := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?sortBy=createdAt-desc", nil)
	if res.Status != http.StatusOK {
		t.Fatalf("audit list status = %d, body=%s", res.Status, res.Body)
	}
	var logs []map[string]any
	res.JSON(t, &logs)
	adminID := env.UserID(t, "admin")
	found := false
	for _, item := range logs {
		user, _ := item["user"].(map[string]any)
		if item["action"] == "role.create" && user["id"] == adminID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("role.create audit by admin not found: %s", res.Body)
	}
	future := url.QueryEscape("2099-01-01")
	filtered := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?createdAtStart="+future, nil)
	if filtered.Status != http.StatusOK {
		t.Fatalf("future audit filter status = %d, body=%s", filtered.Status, filtered.Body)
	}
	if err := json.Unmarshal(filtered.Body, &logs); err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("future audit filter returned %d logs", len(logs))
	}
}
