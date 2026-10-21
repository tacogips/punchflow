package contract_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var liveUsersGradesOperations = []string{
	"createUser", "deleteUser", "updateUser", "lookupUserByEmail", "reinviteUser",
	"suspendUser", "reactivateUser", "listUserTeams", "listUserRoles",
	"createGrade", "deleteGrade", "updateGrade", "setDefaultGrade",
}

func TestContractLiveUsersGrades(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	ops := make(map[string]apiv1.Operation)
	for _, op := range apiv1.Operations() {
		ops[op.OperationID] = op
	}
	schemas := make(map[string]*jsonschema.Schema)
	passed := make(map[string]bool, len(liveUsersGradesOperations))

	run := func(operationID string, fn func(*testing.T)) {
		passed[operationID] = t.Run(operationID, fn)
	}
	runError := func(operationID, code string, response *testutil.Response, status int) {
		t.Run(operationID+"/error_"+code, func(t *testing.T) {
			pf26cCheckError(t, compiler, response, status, code)
		})
	}

	var userID string
	run("createUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{
			"email": "pf26c-new@punchflow.test", "code": "pf26c-new", "firstName": strings.Repeat("A", 127), "lastName": strings.Repeat("B", 127),
		})
		pf26cCheck(t, "createUser", response, ops, spec, compiler, schemas)
		var user map[string]any
		if err := json.Unmarshal(response.Body, &user); err != nil {
			t.Fatalf("decode created user: %v", err)
		}
		if user["status"] != "invited" {
			t.Errorf("created user status=%v, want invited", user["status"])
		}
		if len([]rune(user["fullName"].(string))) != 255 {
			t.Errorf("created populated fullName length=%d, want schema maxLength 255", len([]rune(user["fullName"].(string))))
		}
		userID, _ = user["id"].(string)
		if userID == "" {
			t.Fatal("created user has no id")
		}
	})
	run("updateUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+url.PathEscape(userID), map[string]any{"firstName": "PF26c Updated"})
		pf26cCheck(t, "updateUser", response, ops, spec, compiler, schemas)
	})
	run("lookupUserByEmail", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/lookupByEmail?email="+url.QueryEscape("sales-rep@punchflow.test"), nil)
		pf26cCheck(t, "lookupUserByEmail", response, ops, spec, compiler, schemas)
	})
	run("reinviteUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+url.PathEscape(userID)+"/reinvite", nil)
		pf26cCheck(t, "reinviteUser", response, ops, spec, compiler, schemas)
	})
	run("suspendUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/engineer/suspend", nil)
		pf26cCheck(t, "suspendUser", response, ops, spec, compiler, schemas)
		pf26cAssertStatus(t, response.Body, "suspended")
	})
	response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/invited-user/suspend", nil)
	runError("suspendUser", "invalid_state", response, http.StatusUnprocessableEntity)
	run("reactivateUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/engineer/reactivate", nil)
		pf26cCheck(t, "reactivateUser", response, ops, spec, compiler, schemas)
		pf26cAssertStatus(t, response.Body, "activated")
	})
	run("listUserTeams", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/sales-rep/teams", nil)
		pf26cCheck(t, "listUserTeams", response, ops, spec, compiler, schemas)

		linkUser := "sales-rep"
		linkResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+linkUser+"/teams?perPage=1", nil)
		pf26cCheck(t, "listUserTeams", linkResponse, ops, spec, compiler, schemas)
		total, err := strconv.Atoi(linkResponse.Header.Get("Total"))
		if err != nil {
			t.Fatalf("parse listUserTeams Total header %q: %v", linkResponse.Header.Get("Total"), err)
		}
		if total <= 1 {
			for _, candidate := range []string{"admin", "engineer", "accountant", "sales-manager"} {
				candidateResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+candidate+"/teams?perPage=1", nil)
				pf26cCheck(t, "listUserTeams", candidateResponse, ops, spec, compiler, schemas)
				candidateTotal, parseErr := strconv.Atoi(candidateResponse.Header.Get("Total"))
				if parseErr != nil {
					t.Fatalf("parse listUserTeams Total header for %s: %q: %v", candidate, candidateResponse.Header.Get("Total"), parseErr)
				}
				if candidateTotal > 1 {
					linkUser, linkResponse, total = candidate, candidateResponse, candidateTotal
					break
				}
			}
		}
		t.Logf("listUserTeams Link rel=next check uses user %s with Total=%d", linkUser, total)
		if total <= 1 {
			t.Fatalf("no fixture user has more than one team for Link rel=next check; last user=%s Total=%d", linkUser, total)
		}
		if !strings.Contains(linkResponse.Header.Get("Link"), `rel="next"`) {
			t.Errorf("listUserTeams Link=%q, want rel=\"next\"", linkResponse.Header.Get("Link"))
		}
	})
	run("listUserRoles", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/admin/roles", nil)
		pf26cCheck(t, "listUserRoles", response, ops, spec, compiler, schemas)
	})
	response = env.Do(t, testutil.Employee, http.MethodGet, "/v1/users/admin/roles", nil)
	runError("listUserRoles", "missing_permission", response, http.StatusForbidden)
	run("deleteUser", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+url.PathEscape(userID), nil)
		pf26cCheck(t, "deleteUser", response, ops, spec, compiler, schemas)
		pf26cAssertStatus(t, response.Body, "deactivated")
	})
	response = env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/00000000-0000-4000-8000-000000000000", nil)
	runError("deleteUser", "resource_not_found", response, http.StatusNotFound)

	run("createGrade", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"code": "pf26c-grade", "level": 60, "name": "PF26c Grade"})
		pf26cCheck(t, "createGrade", response, ops, spec, compiler, schemas)
		temporary := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"code": "pf26c-temp", "level": 70, "name": "PF26c Temporary"})
		pf26cCheck(t, "createGrade", temporary, ops, spec, compiler, schemas)
	})
	response = env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"code": "pf26c-duplicate-level", "level": 10, "name": "PF26c Duplicate Level"})
	runError("createGrade", "validation_failed", response, http.StatusUnprocessableEntity)
	run("updateGrade", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/grades/pf26c-grade", map[string]any{"name": "PF26c Grade Updated"})
		pf26cCheck(t, "updateGrade", response, ops, spec, compiler, schemas)
	})
	run("setDefaultGrade", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades/pf26c-grade/default", nil)
		pf26cCheck(t, "setDefaultGrade", response, ops, spec, compiler, schemas)
	})
	run("deleteGrade", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/grades/pf26c-temp", nil)
		pf26cCheck(t, "deleteGrade", response, ops, spec, compiler, schemas)
	})

	if len(liveUsersGradesOperations) != 13 {
		t.Errorf("live users/grades inventory has %d operations, want 13", len(liveUsersGradesOperations))
	}
	for _, operationID := range liveUsersGradesOperations {
		if _, ok := ops[operationID]; !ok {
			t.Errorf("live users/grades operation %s is not registered in apiv1.Operations()", operationID)
		}
		if !passed[operationID] {
			t.Errorf("live users/grades operation %s has no passing operation subtest", operationID)
		}
	}
}

func pf26cCheck(t *testing.T, operationID string, response *testutil.Response, ops map[string]apiv1.Operation, spec map[string]any, compiler *jsonschema.Compiler, cache map[string]*jsonschema.Schema) {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", operationID, response.Status, response.Body)
	}
	op, ok := ops[operationID]
	if !ok {
		t.Fatalf("operation %s is not registered", operationID)
	}
	pathItem, ok := spec["paths"].(map[string]any)[op.Pattern].(map[string]any)
	if !ok {
		t.Fatalf("spec has no path %s for %s", op.Pattern, operationID)
	}
	operation, ok := pathItem[strings.ToLower(op.Method)].(map[string]any)
	if !ok {
		t.Fatalf("spec has no %s operation for %s", strings.ToLower(op.Method), operationID)
	}
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("spec has no responses for %s", operationID)
	}
	responseSpec, ok := responses["200"].(map[string]any)
	if !ok {
		responseSpec, ok = responses["200.0"].(map[string]any)
	}
	if !ok {
		t.Fatalf("spec has no 200 response for %s", operationID)
	}
	content, _ := responseSpec["content"].(map[string]any)
	if content == nil {
		if len(response.Body) != 0 {
			t.Errorf("%s declares an empty response, got body=%s", operationID, response.Body)
		}
	} else {
		uri := responseSchemaURI(op.Pattern, strings.ToLower(op.Method))
		schema := cache[uri]
		if schema == nil {
			var err error
			schema, err = compiler.Compile(uri)
			if err != nil {
				t.Fatalf("compile %s schema %s: %v", operationID, uri, err)
			}
			cache[uri] = schema
		}
		validateJSON(t, schema, response.Body)
		var decoded any
		if err := json.Unmarshal(response.Body, &decoded); err != nil {
			t.Fatalf("decode %s response: %v", operationID, err)
		}
		checkTimestamps(t, decoded)
	}
	if pf26cHasPerPage(operation) || pf26cHasPerPage(pathItem) {
		for _, header := range []string{"Page", "Per-Page", "Total"} {
			if response.Header.Get(header) == "" {
				t.Errorf("%s response missing %s pagination header", operationID, header)
			}
		}
	}
}

func pf26cCheckError(t *testing.T, compiler *jsonschema.Compiler, response *testutil.Response, status int, code string) {
	t.Helper()
	testutil.AssertError(t, response, status, code)
	const uri = "file:///kickflow-openapi.json#/components/schemas/ErrorResponse"
	schema, err := compiler.Compile(uri)
	if err != nil {
		t.Fatalf("compile ErrorResponse schema: %v", err)
	}
	validateJSON(t, schema, response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode %s error response: %v", code, err)
	}
	for key := range body {
		if key != "code" && key != "message" && key != "errors" {
			t.Errorf("%s error response contains unexpected key %q", code, key)
		}
	}
	_, hasErrors := body["errors"]
	if hasErrors && status != http.StatusUnprocessableEntity {
		t.Errorf("%s error response contains errors for status=%d", code, status)
	}
}

func pf26cAssertStatus(t *testing.T, data []byte, want string) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, _ := body["status"].(string); got != want {
		t.Errorf("response status=%v, want %s", body["status"], want)
	}
}

func pf26cHasPerPage(value any) bool {
	container, _ := value.(map[string]any)
	parameters, _ := container["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["name"] == "perPage" {
			return true
		}
	}
	return false
}
