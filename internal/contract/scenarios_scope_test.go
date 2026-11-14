package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestContractLiveScopedAndScheduledShapes validates response shapes that only
// appear in specific states: a scheduled organization chart activation, users
// with custom fields (visible and administrator views), restricted role
// details, and the empty body of a recursive folder delete.
func TestContractLiveScopedAndScheduledShapes(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	schemas := make(map[string]*jsonschema.Schema)
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	decode := func(t *testing.T, response *testutil.Response) map[string]any {
		t.Helper()
		var body map[string]any
		if err := json.Unmarshal(response.Body, &body); err != nil {
			t.Fatalf("decode response: %v (%s)", err, response.Body)
		}
		return body
	}

	t.Run("activateOrganizationChart/scheduled", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/00000000-0000-4000-8000-000000000028/activate", map[string]any{"dueOn": "2026-02-01"})
		pf26dCheck(t, spec, compiler, schemas, operations["activateOrganizationChart"], response)
		if plan, _ := decode(t, response)["activationPlan"].(map[string]any); plan == nil || plan["dueOn"] != "2026-02-01" {
			t.Fatalf("activationPlan = %v, want a plan due 2026-02-01", plan)
		}
		pf26dCheck(t, spec, compiler, schemas, operations["listOrganizationCharts"], env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts", nil))
	})

	t.Run("users/customFields", func(t *testing.T) {
		admin := env.UserID(t, "admin")
		administrator := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+admin, nil)
		pf26dCheck(t, spec, compiler, schemas, operations["getUser"], administrator)
		if fields, _ := decode(t, administrator)["customFields"].([]any); len(fields) != 3 {
			t.Fatalf("administrator view custom fields = %v, want 3", fields)
		}
		created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{
			"email": "contract-custom@punchflow.test", "firstName": "Custom", "lastName": "Fields",
			"customFields": []map[string]any{{"code": "employment-type", "value": "contractor"}, {"code": "skills", "value": []string{"go"}}},
		})
		pf26dCheck(t, spec, compiler, schemas, operations["createUser"], created)
		pf26dCheck(t, spec, compiler, schemas, operations["listUsers"], env.Do(t, testutil.Admin, http.MethodGet, "/v1/users?perPage=100", nil))
		pf26dCheck(t, spec, compiler, schemas, operations["getCurrentUser"], env.Do(t, testutil.Employee, http.MethodGet, "/v1/user", nil))
	})

	t.Run("roles/restricted", func(t *testing.T) {
		created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{
			"name": "Contract restricted", "permissionList": []any{map[string]any{
				"permission": "master", "restricted": true, "folderIds": []string{}, "teamIds": []string{},
				"generalMasterIds": []string{"00000000-0000-4000-8000-000000000039"},
			}},
		})
		pf26dCheck(t, spec, compiler, schemas, operations["createRole"], created)
		roleID, _ := decode(t, created)["id"].(string)
		pf26dCheck(t, spec, compiler, schemas, operations["getRole"], env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID, nil))
	})

	t.Run("deleteFolder/recursive", func(t *testing.T) {
		parent := decode(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{"name": "Contract parent"}))
		child := env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{"name": "Contract child", "parentFolderId": parent["id"]})
		pf26dCheck(t, spec, compiler, schemas, operations["createFolder"], child)
		pf26dCheck(t, spec, compiler, schemas, operations["deleteFolder"], env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/"+parent["id"].(string), nil))
		testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/folders/"+parent["id"].(string), nil), http.StatusNotFound, "resource_not_found")
	})
}
