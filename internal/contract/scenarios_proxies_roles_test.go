package contract_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"
)

var liveProxiesRolesOperations = []string{
	"createProxyApplicant", "updateProxyApplicant", "deleteProxyApplicant",
	"createProxyApprover", "updateProxyApprover", "deleteProxyApprover",
	"createRole", "getRole", "updateRole", "deleteRole",
	"createRoleMembers", "listRoleMembers", "deleteRoleMember",
}

func TestContractLiveProxiesRoles(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	ops := make(map[string]apiv1.Operation)
	for _, op := range apiv1.Operations() {
		ops[op.OperationID] = op
	}
	passed := make(map[string]bool, len(liveProxiesRolesOperations))
	for _, id := range liveProxiesRolesOperations {
		if _, ok := ops[id]; !ok {
			t.Errorf("operation %s is not registered", id)
		}
		passed[id] = true
	}

	pf26fRun := func(id, name string, run func(*testing.T)) {
		if !t.Run(name, run) {
			passed[id] = false
		}
	}
	pf26fCheck := func(t *testing.T, id string, response *testutil.Response) map[string]any {
		t.Helper()
		op, ok := ops[id]
		if !ok {
			t.Fatalf("operation %s is not registered", id)
		}
		if response.Status != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", id, response.Status, response.Body)
		}
		paths := spec["paths"].(map[string]any)
		pathItem := paths[op.Pattern].(map[string]any)
		operation := pathItem[strings.ToLower(op.Method)].(map[string]any)
		responses := operation["responses"].(map[string]any)
		responseSpec, ok := responses["200"].(map[string]any)
		if !ok {
			responseSpec, ok = responses["200.0"].(map[string]any)
		}
		if !ok {
			t.Fatalf("%s has no 200 schema", id)
		}
		content, _ := responseSpec["content"].(map[string]any)
		if content == nil {
			if len(response.Body) != 0 {
				t.Fatalf("%s response should be empty, got %s", id, response.Body)
			}
			return nil
		}
		jsonContent, _ := content["application/json"].(map[string]any)
		if jsonContent == nil {
			t.Fatalf("%s has no application/json response", id)
		}
		uri := responseSchemaURI(op.Pattern, strings.ToLower(op.Method))
		schema, err := compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile %s schema %s: %v", id, uri, err)
		}
		validateJSON(t, schema, response.Body)
		var decoded map[string]any
		if err := json.Unmarshal(response.Body, &decoded); err == nil {
			checkTimestamps(t, decoded)
		} else {
			var value any
			if err := json.Unmarshal(response.Body, &value); err != nil {
				t.Fatalf("decode %s response: %v", id, err)
			}
			checkTimestamps(t, value)
		}
		parameters, _ := operation["parameters"].([]any)
		for _, raw := range parameters {
			parameter, _ := raw.(map[string]any)
			if parameter["name"] == "perPage" {
				for _, header := range []string{"Page", "Per-Page", "Total"} {
					if response.Header.Get(header) == "" {
						t.Errorf("%s response missing %s pagination header", id, header)
					}
				}
				break
			}
		}
		return decoded
	}
	pf26fCheckError := func(t *testing.T, response *testutil.Response, status int, code string) {
		t.Helper()
		testutil.AssertError(t, response, status, code)
		errorSchema, err := compiler.Compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
		if err != nil {
			t.Fatalf("compile ErrorResponse schema: %v", err)
		}
		validateJSON(t, errorSchema, response.Body)
		var body map[string]any
		if err := json.Unmarshal(response.Body, &body); err != nil {
			t.Fatal(err)
		}
		for key := range body {
			if key != "code" && key != "message" && key != "errors" {
				t.Errorf("unexpected error key %q", key)
			}
		}
		_, hasErrors := body["errors"]
		if hasErrors != (status == http.StatusUnprocessableEntity) {
			t.Errorf("errors presence=%t for status %d", hasErrors, status)
		}
	}

	workflowResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if workflowResponse.Status != http.StatusOK {
		t.Fatalf("get expense workflow status=%d body=%s", workflowResponse.Status, workflowResponse.Body)
	}
	var workflow map[string]any
	workflowResponse.JSON(t, &workflow)
	workflowID, _ := workflow["id"].(string)
	if workflowID == "" {
		t.Fatal("expense workflow has no id")
	}

	var applicantID, approverID, roleID string
	applicantBody := map[string]any{"userId": env.UserID(t, "engineer"), "proxyUserId": env.UserID(t, "eng-manager"), "workflowIds": []string{}}
	pf26fRun("createProxyApplicant", "createProxyApplicant", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", applicantBody)
		decoded := pf26fCheck(t, "createProxyApplicant", response)
		applicantID, _ = decoded["id"].(string)
	})
	pf26fRun("createProxyApplicant", "createProxyApplicant/error_validation_failed", func(t *testing.T) {
		body := map[string]any{"userId": env.UserID(t, "ceo"), "proxyUserId": env.UserID(t, "assistant"), "workflowIds": []string{}}
		pf26fCheckError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", body), http.StatusUnprocessableEntity, "validation_failed")
	})
	pf26fRun("updateProxyApplicant", "updateProxyApplicant", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/proxyApplicants/"+applicantID, map[string]any{"workflowIds": []string{workflowID, workflowID}})
		decoded := pf26fCheck(t, "updateProxyApplicant", response)
		workflows, _ := decoded["workflows"].([]any)
		if len(workflows) != 1 {
			t.Errorf("deduplicated workflows length=%d, want 1", len(workflows))
		}
	})
	pf26fRun("deleteProxyApplicant", "deleteProxyApplicant", func(t *testing.T) {
		pf26fCheck(t, "deleteProxyApplicant", env.Do(t, testutil.Admin, http.MethodDelete, "/v1/proxyApplicants/"+applicantID, nil))
	})
	approverBody := map[string]any{"userId": env.UserID(t, "finance-manager"), "proxyUserId": env.UserID(t, "accountant"), "workflowIds": []string{}}
	pf26fRun("createProxyApprover", "createProxyApprover", func(t *testing.T) {
		decoded := pf26fCheck(t, "createProxyApprover", env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApprovers", approverBody))
		approverID, _ = decoded["id"].(string)
	})
	pf26fRun("createProxyApprover", "createProxyApprover/error_missing_permission", func(t *testing.T) {
		pf26fCheckError(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/proxyApprovers", approverBody), http.StatusForbidden, "missing_permission")
	})
	pf26fRun("updateProxyApprover", "updateProxyApprover", func(t *testing.T) {
		pf26fCheck(t, "updateProxyApprover", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/proxyApprovers/"+approverID, map[string]any{"workflowIds": []string{workflowID}}))
	})
	pf26fRun("deleteProxyApprover", "deleteProxyApprover", func(t *testing.T) {
		pf26fCheck(t, "deleteProxyApprover", env.Do(t, testutil.Admin, http.MethodDelete, "/v1/proxyApprovers/"+approverID, nil))
	})

	roleBody := map[string]any{"name": "PF-26f contract role", "permissionList": []any{map[string]any{"permission": "audit", "restricted": false}}}
	pf26fRun("createRole", "createRole", func(t *testing.T) {
		decoded := pf26fCheck(t, "createRole", env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", roleBody))
		roleID, _ = decoded["id"].(string)
	})
	pf26fRun("getRole", "getRole", func(t *testing.T) {
		pf26fCheck(t, "getRole", env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID, nil))
	})
	pf26fRun("getRole", "getRole/error_resource_not_found", func(t *testing.T) {
		pf26fCheckError(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/00000000-0000-4000-8000-000000000099", nil), http.StatusNotFound, "resource_not_found")
	})
	pf26fRun("updateRole", "updateRole", func(t *testing.T) {
		pf26fCheck(t, "updateRole", env.Do(t, testutil.Admin, http.MethodPatch, "/v1/roles/"+roleID, map[string]any{"name": "PF-26f updated role"}))
	})
	pf26fRun("updateRole", "updateRole/error_resource_not_editable", func(t *testing.T) {
		pf26fCheckError(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/roles/00000000-0000-4000-8000-000000000029", map[string]any{"name": "Forbidden change"}), http.StatusForbidden, "resource_not_editable")
	})
	users := []string{env.UserID(t, "engineer"), env.UserID(t, "accountant")}
	pf26fRun("createRoleMembers", "createRoleMembers", func(t *testing.T) {
		pf26fCheck(t, "createRoleMembers", env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+roleID+"/memberships", map[string]any{"userIds": users}))
	})
	pf26fRun("listRoleMembers", "listRoleMembers", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID+"/memberships", nil)
		pf26fCheck(t, "listRoleMembers", response)
		linkResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+roleID+"/memberships?perPage=1", nil)
		pf26fCheck(t, "listRoleMembers", linkResponse)
		if !strings.Contains(linkResponse.Header.Get("Link"), `rel="next"`) {
			t.Errorf("Link header %q does not contain rel=next", linkResponse.Header.Get("Link"))
		}
		var members []any
		if err := json.Unmarshal(linkResponse.Body, &members); err != nil {
			t.Fatal(err)
		}
		if len(members) != 1 {
			t.Errorf("perPage=1 returned %d members, want 1", len(members))
		}
	})
	pf26fRun("deleteRoleMember", "deleteRoleMember", func(t *testing.T) {
		pf26fCheck(t, "deleteRoleMember", env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+roleID+"/memberships/"+users[0], nil))
	})
	pf26fRun("deleteRoleMember", "deleteRoleMember/error_validation_failed", func(t *testing.T) {
		pf26fCheckError(t, env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/00000000-0000-4000-8000-000000000029/memberships/"+env.UserID(t, "admin"), nil), http.StatusUnprocessableEntity, "validation_failed")
	})
	pf26fRun("deleteRole", "deleteRole", func(t *testing.T) {
		pf26fCheck(t, "deleteRole", env.Do(t, testutil.Admin, http.MethodDelete, "/v1/roles/"+roleID, nil))
	})

	if len(liveProxiesRolesOperations) != 13 {
		t.Errorf("operation inventory has %d entries, want 13", len(liveProxiesRolesOperations))
	}
	for _, id := range liveProxiesRolesOperations {
		if !passed[id] {
			t.Errorf("operation %s has no passing live subtest", id)
		}
	}
}
