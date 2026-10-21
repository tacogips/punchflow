package contract_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"
)

func TestContractScenarios(t *testing.T) {
	spec := loadSpec(t)
	c := schemaCompiler(t, spec)
	paths := spec["paths"].(map[string]any)
	for _, op := range apiv1.Operations() {
		op := op
		t.Run(op.OperationID, func(t *testing.T) {
			pathItem := paths[op.Pattern].(map[string]any)
			operation := pathItem[strings.ToLower(op.Method)].(map[string]any)
			responses := operation["responses"].(map[string]any)
			response, ok := responses["200"].(map[string]any)
			if !ok {
				response, ok = responses["200.0"].(map[string]any)
			}
			if !ok {
				t.Fatalf("%s has no 200 response", op.OperationID)
			}
			content, _ := response["content"].(map[string]any)
			jsonContent, _ := content["application/json"].(map[string]any)
			if jsonContent == nil {
				// The operation contract explicitly declares an empty response.
				if content == nil {
					return
				}
				t.Fatalf("%s has no application/json response schema", op.OperationID)
			}
			if _, ok := jsonContent["schema"]; !ok {
				t.Fatalf("%s response schema missing", op.OperationID)
			}
			uri := responseSchemaURI(op.Pattern, strings.ToLower(op.Method))
			if _, err := c.Compile(uri); err != nil {
				t.Fatalf("compile %s schema %s: %v", op.OperationID, uri, err)
			}
		})
	}
	if len(apiv1.Operations()) != 96 {
		t.Fatal("scenario inventory unexpectedly changed")
	}
}

var liveReadScenarios = []struct{ operationID, path string }{
	{"getCurrentUser", "/v1/user"}, {"listUsers", "/v1/users"}, {"getUser", "/v1/users/sales-rep"},
	{"listGrades", "/v1/grades"}, {"getGrade", "/v1/grades/manager"}, {"listProxyApplicants", "/v1/proxyApplicants"},
	{"listProxyApprovers", "/v1/proxyApprovers"}, {"listOrganizationCharts", "/v1/organizationCharts"},
	{"getCurrentOrganizationChart", "/v1/organizationChart"}, {"listCategories", "/v1/categories"},
	{"listFolders", "/v1/folders"}, {"getFolder", "/v1/folders/finance"}, {"listRoutes", "/v1/routes"},
	{"getRoute", "/v1/routes/expense-standard"}, {"listWorkflows", "/v1/workflows"},
	{"getWorkflow", "/v1/workflows/expense-claim"}, {"listTickets", "/v1/tickets"}, {"listTasks", "/v1/tickets/tasks"},
	{"listGeneralMasters", "/v1/generalMasters"}, {"getGeneralMaster", "/v1/generalMasters/cost-centers"},
	{"listRoles", "/v1/roles"}, {"listAuditLogs", "/v1/auditLogs"},
}

func TestContractLiveReadScenarios(t *testing.T) {
	env := testutil.New(t)
	ops := make(map[string]apiv1.Operation)
	for _, op := range apiv1.Operations() {
		ops[op.OperationID] = op
	}
	spec := loadSpec(t)
	c := schemaCompiler(t, spec)
	for _, item := range liveReadScenarios {
		item := item
		t.Run(item.operationID, func(t *testing.T) {
			op, ok := ops[item.operationID]
			if !ok {
				t.Fatalf("operation %s is not registered", item.operationID)
			}
			response := env.Do(t, testutil.Admin, http.MethodGet, item.path, nil)
			if response.Status != http.StatusOK {
				t.Fatalf("GET %s status=%d body=%s", item.path, response.Status, response.Body)
			}
			schema, err := c.Compile(responseSchemaURI(op.Pattern, strings.ToLower(op.Method)))
			if err != nil {
				t.Fatalf("compile %s schema: %v", item.operationID, err)
			}
			validateJSON(t, schema, response.Body)
			var decoded any
			if err := json.Unmarshal(response.Body, &decoded); err != nil {
				t.Fatal(err)
			}
			checkTimestamps(t, decoded)
			if _, isArray := decoded.([]any); isArray {
				for _, header := range []string{"Page", "Per-Page", "Total"} {
					if response.Header.Get(header) == "" {
						t.Errorf("%s response missing %s pagination header", item.operationID, header)
					}
				}
			}
		})
	}
}
