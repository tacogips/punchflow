package contract_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var liveOrgChartsTeamsOperations = []string{
	"createOrganizationChart", "getOrganizationChart", "updateOrganizationChart", "createTeam", "getTeam", "listTeams",
	"createTeamMembers", "listTeamMembers", "updateTeamMember", "deleteTeamMembers", "updateTeam", "deleteTeam",
	"activateOrganizationChart", "deleteOrganizationChart",
}

func TestContractLiveOrgChartsTeams(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	schemas := make(map[string]*jsonschema.Schema)
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	if len(liveOrgChartsTeamsOperations) != 14 {
		t.Fatalf("live operation inventory has %d entries, want 14", len(liveOrgChartsTeamsOperations))
	}
	passed := make(map[string]bool, len(liveOrgChartsTeamsOperations))
	for _, operationID := range liveOrgChartsTeamsOperations {
		if _, ok := operations[operationID]; !ok {
			t.Errorf("live scenario operation %q is not registered", operationID)
		}
	}

	chartPath := "/v1/organizationCharts"
	var chartID, disposableChartID, childID, rootID string
	rootPath := ""
	childPath := ""
	gradeID := "00000000-0000-4000-8000-000000000006"
	for _, operationID := range liveOrgChartsTeamsOperations {
		if t.Run(operationID, func(t *testing.T) {
			operation := operations[operationID]
			switch operationID {
			case "createOrganizationChart":
				response := env.Do(t, testutil.Admin, http.MethodPost, chartPath, map[string]any{"name": "PF26d chart"})
				pf26dCheck(t, spec, compiler, schemas, operation, response)
				var body map[string]any
				if err := json.Unmarshal(response.Body, &body); err != nil {
					t.Fatalf("decode created organization chart: %v", err)
				}
				chartID, _ = body["id"].(string)
				if chartID == "" {
					t.Fatal("created organization chart has no id")
				}
				if body["current"] != false {
					t.Errorf("new organization chart current = %v, want false", body["current"])
				}
				disposable := env.Do(t, testutil.Admin, http.MethodPost, chartPath, map[string]any{"name": "PF26d disposable chart"})
				pf26dCheck(t, spec, compiler, schemas, operation, disposable)
				var disposableBody map[string]any
				if err := json.Unmarshal(disposable.Body, &disposableBody); err != nil {
					t.Fatalf("decode disposable organization chart: %v", err)
				}
				disposableChartID, _ = disposableBody["id"].(string)
			case "getOrganizationChart":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodGet, chartPath+"/"+chartID, nil))
			case "updateOrganizationChart":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPatch, chartPath+"/"+chartID, map[string]any{"name": "PF26d updated chart"}))
			case "createTeam":
				teamPath := chartPath + "/" + chartID + "/teams"
				rootPath = teamPath + "/pf26d-root"
				childPath = teamPath + "/pf26d-child"
				root := env.Do(t, testutil.Admin, http.MethodPost, teamPath, map[string]any{"name": "PF26d root", "code": "pf26d-root"})
				pf26dCheck(t, spec, compiler, schemas, operation, root)
				var rootBody map[string]any
				if err := json.Unmarshal(root.Body, &rootBody); err != nil {
					t.Fatalf("decode created root team: %v", err)
				}
				rootID, _ = rootBody["id"].(string)
				child := env.Do(t, testutil.Admin, http.MethodPost, teamPath, map[string]any{"name": "PF26d child", "code": "pf26d-child", "parentId": "pf26d-root"})
				pf26dCheck(t, spec, compiler, schemas, operation, child)
				var body map[string]any
				if err := json.Unmarshal(child.Body, &body); err != nil {
					t.Fatalf("decode created child team: %v", err)
				}
				childID, _ = body["id"].(string)
			case "getTeam":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodGet, rootPath, nil))
			case "listTeams":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodGet, strings.TrimSuffix(rootPath, "/pf26d-root"), nil))
			case "createTeamMembers":
				body := map[string]any{"users": []any{
					map[string]any{"id": env.UserID(t, "sales-manager"), "leader": true, "gradeIds": []string{gradeID}},
					map[string]any{"id": env.UserID(t, "sales-rep"), "leader": false, "gradeIds": []string{gradeID}},
					map[string]any{"id": env.UserID(t, "sales-deputy"), "leader": false, "gradeIds": []string{gradeID}},
				}}
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPost, rootPath+"/memberships", body))
				// Omitting gradeIds on creation uses the configured default grade.
				defaultGradeUser := env.UserID(t, "engineer")
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPost, rootPath+"/memberships", map[string]any{"users": []any{
					map[string]any{"id": defaultGradeUser, "leader": false},
				}}))
				members, err := store.GetTeamMemberships(t.Context(), env.DB.Q(), rootID)
				if err != nil {
					t.Fatal(err)
				}
				defaultGrade, err := store.DefaultGradeID(t.Context(), env.DB.Q())
				if err != nil {
					t.Fatal(err)
				}
				foundDefault := false
				for _, member := range members {
					if member.UserID == defaultGradeUser && len(member.GradeIDs) == 1 && member.GradeIDs[0] == defaultGrade {
						foundDefault = true
					}
				}
				if !foundDefault {
					t.Fatal("created membership did not receive the configured default grade")
				}
				t.Run("error_empty_grade_ids", func(t *testing.T) {
					response := env.Do(t, testutil.Admin, http.MethodPost, rootPath+"/memberships", map[string]any{"users": []any{
						map[string]any{"id": env.UserID(t, "accountant"), "leader": false, "gradeIds": []string{}},
					}})
					pf26dCheckError(t, compiler, schemas, response, http.StatusUnprocessableEntity, "validation_failed")
				})
				t.Run("error_validation_failed", func(t *testing.T) {
					response := env.Do(t, testutil.Admin, http.MethodPost, childPath+"/memberships", map[string]any{"users": []any{
						map[string]any{"id": env.UserID(t, "sales-rep"), "leader": false, "gradeIds": []string{gradeID}},
					}})
					pf26dCheckError(t, compiler, schemas, response, http.StatusUnprocessableEntity, "validation_failed")
				})
			case "listTeamMembers":
				path := rootPath + "/memberships"
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodGet, path, nil))
				page := env.Do(t, testutil.Admin, http.MethodGet, path+"?perPage=1", nil)
				pf26dCheck(t, spec, compiler, schemas, operation, page)
				if !strings.Contains(page.Header.Get("Link"), `rel="next"`) {
					t.Errorf("Link header = %q, want rel=next", page.Header.Get("Link"))
				}
			case "updateTeamMember":
				path := rootPath + "/memberships/" + env.UserID(t, "sales-rep")
				t.Run("error_missing_required_fields", func(t *testing.T) {
					response := env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{})
					pf26dCheckError(t, compiler, schemas, response, http.StatusBadRequest, "invalid_parameter")
				})
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{"leader": true, "gradeIds": []string{gradeID}}))
			case "deleteTeamMembers":
				path := rootPath + "/memberships"
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodDelete, path, map[string]any{"userIds": []string{env.UserID(t, "sales-manager")}}))
				t.Run("error_validation_failed", func(t *testing.T) {
					response := env.Do(t, testutil.Admin, http.MethodDelete, path, map[string]any{"userIds": []string{env.UserID(t, "sales-rep")}})
					pf26dCheckError(t, compiler, schemas, response, http.StatusUnprocessableEntity, "validation_failed")
				})
			case "updateTeam":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPatch, childPath, map[string]any{"name": "PF26d renamed child"}))
				t.Run("error_validation_failed", func(t *testing.T) {
					response := env.Do(t, testutil.Admin, http.MethodPatch, rootPath, map[string]any{"parentId": childID})
					pf26dCheckError(t, compiler, schemas, response, http.StatusUnprocessableEntity, "validation_failed")
				})
			case "deleteTeam":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodDelete, childPath, nil))
			case "activateOrganizationChart":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodPost, chartPath+"/"+chartID+"/activate", nil))
			case "deleteOrganizationChart":
				pf26dCheck(t, spec, compiler, schemas, operation, env.Do(t, testutil.Admin, http.MethodDelete, chartPath+"/"+disposableChartID, nil))
				t.Run("error_validation_failed", func(t *testing.T) {
					pf26dCheckError(t, compiler, schemas, env.Do(t, testutil.Admin, http.MethodDelete, chartPath+"/"+chartID, nil), http.StatusUnprocessableEntity, "validation_failed")
				})
			default:
				t.Fatalf("operation %q has no scenario", operationID)
			}
		}) {
			passed[operationID] = true
		}
	}
	for _, operationID := range liveOrgChartsTeamsOperations {
		if !passed[operationID] {
			t.Errorf("operation %q has no passing live subtest", operationID)
		}
	}
}

func pf26dCheck(t *testing.T, spec map[string]any, compiler *jsonschema.Compiler, schemas map[string]*jsonschema.Schema, operation apiv1.Operation, response *testutil.Response) {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", operation.OperationID, response.Status, response.Body)
	}
	pathItem := spec["paths"].(map[string]any)[operation.Pattern].(map[string]any)
	operationSpec := pathItem[strings.ToLower(operation.Method)].(map[string]any)
	responses := operationSpec["responses"].(map[string]any)
	responseSpec, ok := responses["200"].(map[string]any)
	if !ok {
		responseSpec, ok = responses["200.0"].(map[string]any)
	}
	if !ok {
		t.Fatalf("%s has no 200 response in spec", operation.OperationID)
	}
	content, _ := responseSpec["content"].(map[string]any)
	if len(content) == 0 {
		if len(response.Body) != 0 {
			t.Fatalf("%s response body = %q, want empty body", operation.OperationID, response.Body)
		}
		return
	}
	uri := responseSchemaURI(operation.Pattern, strings.ToLower(operation.Method))
	schema := schemas[uri]
	if schema == nil {
		var err error
		schema, err = compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile %s response schema: %v", operation.OperationID, err)
		}
		schemas[uri] = schema
	}
	validateJSON(t, schema, response.Body)
	var decoded any
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		t.Fatalf("decode %s response: %v", operation.OperationID, err)
	}
	checkTimestamps(t, decoded)
	parameters, _ := operationSpec["parameters"].([]any)
	for _, rawParameter := range parameters {
		parameter, _ := rawParameter.(map[string]any)
		if parameter["name"] != "perPage" {
			continue
		}
		for _, header := range []string{"Page", "Per-Page", "Total"} {
			if response.Header.Get(header) == "" {
				t.Errorf("%s response missing %s pagination header", operation.OperationID, header)
			}
		}
		break
	}
}

func pf26dCheckError(t *testing.T, compiler *jsonschema.Compiler, schemas map[string]*jsonschema.Schema, response *testutil.Response, status int, code string) {
	t.Helper()
	testutil.AssertError(t, response, status, code)
	uri := "file:///kickflow-openapi.json#/components/schemas/ErrorResponse"
	schema := schemas[uri]
	if schema == nil {
		var err error
		schema, err = compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile ErrorResponse schema: %v", err)
		}
		schemas[uri] = schema
	}
	validateJSON(t, schema, response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	for key := range body {
		if key != "code" && key != "message" && key != "errors" {
			t.Errorf("error response has unexpected key %q", key)
		}
	}
	_, hasErrors := body["errors"]
	if hasErrors != (status == http.StatusUnprocessableEntity) {
		t.Errorf("error response errors presence=%t for status %d", hasErrors, status)
	}
	if _, exists := body["doc_url"]; exists {
		t.Errorf("error response unexpectedly contains doc_url")
	}
}
