package contract_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var liveTicketsOperations = []string{
	"createTicket",
	"getTicket",
	"updateTicket",
	"updateTicketForm",
	"approveTicket",
	"rejectTicket",
	"denyTicket",
	"withdrawTicket",
	"archiveTicket",
	"listTicketLinks",
	"listViewers",
	"createViewer",
	"deleteViewer",
}

type pf26aTicket struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type pf26aWorkflow struct {
	SectionList []struct {
		FormFields []struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"formFields"`
	} `json:"sectionList"`
}

func TestContractLiveTickets(t *testing.T) {
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	if len(liveTicketsOperations) != 13 {
		t.Fatalf("ticket operation inventory has %d ids, want 13", len(liveTicketsOperations))
	}
	for _, operationID := range liveTicketsOperations {
		if _, ok := operations[operationID]; !ok {
			t.Fatalf("ticket operation %q is not registered", operationID)
		}
	}

	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	schemas := make(map[string]*jsonschema.Schema, len(liveTicketsOperations))
	for _, operationID := range liveTicketsOperations {
		operation := operations[operationID]
		if _, hasBody := pf26aJSONResponse(t, spec, operation); !hasBody {
			continue
		}
		uri := responseSchemaURI(operation.Pattern, strings.ToLower(operation.Method))
		schema, err := compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile %s response schema: %v", operationID, err)
		}
		schemas[operationID] = schema
	}
	errorSchema, err := compiler.Compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
	if err != nil {
		t.Fatalf("compile ErrorResponse schema: %v", err)
	}

	passed := make(map[string]bool, len(liveTicketsOperations))
	run := func(operationID string, scenario func(*testing.T)) {
		passed[operationID] = t.Run(operationID, scenario)
	}

	var draftID string
	run("createTicket", func(t *testing.T) {
		created := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "draft")
		draftID = created.ID
	})

	approveID := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress").ID
	rejectID := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress").ID
	denyID := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress").ID
	withdrawID := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress").ID

	run("getTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+draftID, nil)
		pf26aCheck(t, operations["getTicket"], response, schemas["getTicket"], spec)
		if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_steps SET instruction='Contract reviewer instruction' WHERE ticket_id=? AND step_order=1`, approveID); err != nil {
			t.Fatalf("set populated ticket step instruction: %v", err)
		}
		instructionResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+approveID, nil)
		pf26aCheck(t, operations["getTicket"], instructionResponse, schemas["getTicket"], spec)
		var instructed struct {
			Inputs []struct {
				Value              any              `json:"value"`
				GeneralMasterItems []map[string]any `json:"generalMasterItems"`
				FormField          struct {
					Code string `json:"code"`
				} `json:"formField"`
			} `json:"inputs"`
			Steps []struct {
				Instruction *string `json:"instruction"`
			} `json:"steps"`
		}
		instructionResponse.JSON(t, &instructed)
		foundInstruction := false
		for _, step := range instructed.Steps {
			foundInstruction = foundInstruction || step.Instruction != nil && *step.Instruction == "Contract reviewer instruction"
		}
		if !foundInstruction {
			t.Fatal("populated ticket step instruction was not returned")
		}
		foundCachedMaster := false
		for _, input := range instructed.Inputs {
			if input.FormField.Code != "cost-center" {
				continue
			}
			values, ok := input.Value.([]any)
			if !ok || len(values) != 1 || values[0].(map[string]any)["name"] != "Sales" || len(input.GeneralMasterItems) != 1 {
				t.Fatalf("master input=%+v, want cached value array and separate resolved array", input)
			}
			foundCachedMaster = true
		}
		if !foundCachedMaster {
			t.Fatal("populated master reference value missing from schema-validated ticket detail")
		}
		if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE ticket_assignees SET pending=1 WHERE id=(SELECT ta.id FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id JOIN tickets tk ON tk.id=ts.ticket_id WHERE tk.id=? AND ts.status='in_progress' ORDER BY ta.created_at,ta.id LIMIT 1)`, approveID); err != nil {
			t.Fatalf("set populated assignment hold: %v", err)
		}
		populated := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+approveID, nil)
		pf26aCheck(t, operations["getTicket"], populated, schemas["getTicket"], spec)
		var detail struct {
			Steps []struct {
				Assignees []struct {
					Pending bool `json:"pending"`
				} `json:"assignees"`
			} `json:"steps"`
		}
		populated.JSON(t, &detail)
		foundHeld := false
		for _, step := range detail.Steps {
			for _, assignee := range step.Assignees {
				foundHeld = foundHeld || assignee.Pending
			}
		}
		if !foundHeld {
			t.Fatal("populated ticket detail did not expose a held assignee")
		}
		t.Run("error_resource_not_found", func(t *testing.T) {
			missing := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/00000000-0000-4000-8000-000000000000", nil)
			pf26aCheckError(t, missing, http.StatusNotFound, "resource_not_found", errorSchema)
		})
	})

	run("updateTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+draftID, map[string]any{
			"title": "PF-26a updated ticket",
		})
		pf26aCheck(t, operations["updateTicket"], response, schemas["updateTicket"], spec)
	})

	run("updateTicketForm", func(t *testing.T) {
		fields := pf26aFormFields(t, env)
		response := env.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+approveID+"/form", map[string]any{
			"inputs": []map[string]any{{"formFieldId": fields["purpose"], "value": "Updated purpose"}},
		})
		pf26aCheck(t, operations["updateTicketForm"], response, schemas["updateTicketForm"], spec)
		t.Run("error_invalid_parameter", func(t *testing.T) {
			invalid := env.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+approveID+"/form", map[string]any{})
			pf26aCheckError(t, invalid, http.StatusBadRequest, "invalid_parameter", errorSchema)
		})
	})

	run("approveTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+approveID+"/approve", nil)
		pf26aCheck(t, operations["approveTicket"], response, schemas["approveTicket"], spec)
		t.Run("error_user_not_assigned", func(t *testing.T) {
			unassigned := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+rejectID+"/approve", nil)
			pf26aCheckError(t, unassigned, http.StatusForbidden, "user_not_assigned", errorSchema)
		})
	})

	run("rejectTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejectID+"/reject", map[string]any{"to": 0})
		pf26aCheck(t, operations["rejectTicket"], response, schemas["rejectTicket"], spec)
		pf26aAssertStatus(t, response, "rejected")
	})

	run("denyTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+denyID+"/deny", nil)
		pf26aCheck(t, operations["denyTicket"], response, schemas["denyTicket"], spec)
		pf26aAssertStatus(t, response, "denied")
	})

	run("withdrawTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+withdrawID+"/withdraw", nil)
		pf26aCheck(t, operations["withdrawTicket"], response, schemas["withdrawTicket"], spec)
		pf26aAssertStatus(t, response, "rejected")
	})

	run("listTicketLinks", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+draftID+"/links", nil)
		pf26aCheck(t, operations["listTicketLinks"], response, schemas["listTicketLinks"], spec)
	})

	viewerUserID := env.UserID(t, "engineer")
	financeTeamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "finance")
	if err != nil {
		t.Fatalf("resolve finance team: %v", err)
	}
	run("createViewer", func(t *testing.T) {
		created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+draftID+"/viewers", map[string]any{"userId": viewerUserID})
		pf26aCheck(t, operations["createViewer"], created, schemas["createViewer"], spec)
		second := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+draftID+"/viewers", map[string]any{"teamId": financeTeamID})
		pf26aCheck(t, operations["createViewer"], second, schemas["createViewer"], spec)
		t.Run("error_invalid_parameter", func(t *testing.T) {
			invalid := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+draftID+"/viewers", map[string]any{
				"userId": viewerUserID,
				"teamId": financeTeamID,
			})
			pf26aCheckError(t, invalid, http.StatusBadRequest, "invalid_parameter", errorSchema)
		})
	})

	var viewerID string
	run("listViewers", func(t *testing.T) {
		path := "/v1/tickets/" + draftID + "/viewers"
		response := env.Do(t, testutil.Employee, http.MethodGet, path, nil)
		pf26aCheck(t, operations["listViewers"], response, schemas["listViewers"], spec)
		var viewers []map[string]any
		response.JSON(t, &viewers)
		if len(viewers) < 2 {
			t.Fatalf("list viewers returned %d rows, want at least 2", len(viewers))
		}
		viewerID, _ = viewers[0]["id"].(string)
		if viewerID == "" {
			t.Fatalf("listed viewer has no id: %v", viewers[0])
		}

		page := env.Do(t, testutil.Employee, http.MethodGet, path+"?perPage=1", nil)
		pf26aCheck(t, operations["listViewers"], page, schemas["listViewers"], spec)
		total, parseErr := strconv.Atoi(page.Header.Get("Total"))
		if parseErr != nil || total < 2 {
			t.Fatalf("viewer Total header = %q (parse error %v), want at least 2", page.Header.Get("Total"), parseErr)
		}
		if !strings.Contains(page.Header.Get("Link"), `rel="next"`) {
			t.Errorf("viewer pagination Link = %q, want rel=\"next\"", page.Header.Get("Link"))
		}
	})

	run("deleteViewer", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/tickets/"+draftID+"/viewers/"+viewerID, nil)
		pf26aCheck(t, operations["deleteViewer"], response, schemas["deleteViewer"], spec)
	})

	run("archiveTicket", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+draftID+"/archive", nil)
		pf26aCheck(t, operations["archiveTicket"], response, schemas["archiveTicket"], spec)
		pf26aAssertStatus(t, response, "archived")
		t.Run("error_invalid_state", func(t *testing.T) {
			invalid := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+draftID+"/archive", nil)
			pf26aCheckError(t, invalid, http.StatusUnprocessableEntity, "invalid_state", errorSchema)
		})
	})

	for _, operationID := range liveTicketsOperations {
		if !passed[operationID] {
			t.Errorf("operation %s has no passing live subtest", operationID)
		}
	}
}

func pf26aCreateTicket(t *testing.T, env *testutil.Env, operation apiv1.Operation, schemas map[string]*jsonschema.Schema, spec map[string]any, status string) pf26aTicket {
	t.Helper()
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatalf("resolve expense-claim workflow: %v", err)
	}
	salesTeamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatalf("resolve sales team: %v", err)
	}
	fields := pf26aFormFields(t, env)
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatalf("resolve cost-centers master: %v", err)
	}
	masterItemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatalf("resolve cc-sales master item: %v", err)
	}
	response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": status, "workflowId": workflowID, "authorTeamId": salesTeamID,
		"title": "PF-26a contract ticket",
		"inputs": []map[string]any{
			{"formFieldId": fields["amount"], "value": "5000"},
			{"formFieldId": fields["purpose"], "value": "PF-26a integration test"},
			{"formFieldId": fields["expense-date"], "value": "2026-01-01"},
			{"formFieldId": fields["cost-center"], "generalMasterItemId": masterItemID},
			{"formFieldId": fields["receipt"], "files": []string{}},
			{"formFieldId": fields["approver-note"], "value": ""},
		},
	})
	pf26aCheck(t, operation, response, schemas["createTicket"], spec)
	var ticket pf26aTicket
	response.JSON(t, &ticket)
	if ticket.ID == "" || ticket.Status != status {
		t.Fatalf("created ticket = %+v, want status %q and a non-empty id", ticket, status)
	}
	return ticket
}

func pf26aFormFields(t *testing.T, env *testutil.Env) map[string]string {
	t.Helper()
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get expense-claim workflow: status=%d body=%s", response.Status, response.Body)
	}
	var workflow pf26aWorkflow
	response.JSON(t, &workflow)
	fields := make(map[string]string)
	for _, section := range workflow.SectionList {
		for _, field := range section.FormFields {
			fields[field.Code] = field.ID
		}
	}
	for _, code := range []string{"amount", "purpose", "expense-date", "cost-center", "receipt", "approver-note"} {
		if fields[code] == "" {
			t.Fatalf("expense-claim workflow has no form field %q", code)
		}
	}
	return fields
}

func pf26aCheck(t *testing.T, operation apiv1.Operation, response *testutil.Response, schema *jsonschema.Schema, spec map[string]any) {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", operation.OperationID, response.Status, response.Body)
	}
	_, hasJSON := pf26aJSONResponse(t, spec, operation)
	if !hasJSON {
		if len(response.Body) != 0 {
			t.Fatalf("%s returned %d bytes, want an empty response body: %s", operation.OperationID, len(response.Body), response.Body)
		}
	} else {
		if schema == nil {
			t.Fatalf("%s has a JSON 200 response but no compiled schema", operation.OperationID)
		}
		validateJSON(t, schema, response.Body)
		var decoded any
		if err := json.Unmarshal(response.Body, &decoded); err != nil {
			t.Fatalf("decode %s response: %v", operation.OperationID, err)
		}
		checkTimestamps(t, decoded)
	}
	if pf26aDeclaresPerPage(t, spec, operation) {
		for _, header := range []string{"Page", "Per-Page", "Total"} {
			if response.Header.Get(header) == "" {
				t.Errorf("%s response missing %s pagination header", operation.OperationID, header)
			}
		}
	}
}

func pf26aJSONResponse(t *testing.T, spec map[string]any, operation apiv1.Operation) (map[string]any, bool) {
	t.Helper()
	paths := spec["paths"].(map[string]any)
	pathItem := paths[operation.Pattern].(map[string]any)
	operationSpec := pathItem[strings.ToLower(operation.Method)].(map[string]any)
	responses := operationSpec["responses"].(map[string]any)
	response, ok := responses["200"].(map[string]any)
	if !ok {
		response, ok = responses["200.0"].(map[string]any)
	}
	if !ok {
		t.Fatalf("%s has no 200 response in OpenAPI spec", operation.OperationID)
	}
	content, _ := response["content"].(map[string]any)
	jsonContent, hasJSON := content["application/json"].(map[string]any)
	return jsonContent, hasJSON
}

func pf26aDeclaresPerPage(t *testing.T, spec map[string]any, operation apiv1.Operation) bool {
	t.Helper()
	paths := spec["paths"].(map[string]any)
	pathItem := paths[operation.Pattern].(map[string]any)
	operationSpec := pathItem[strings.ToLower(operation.Method)].(map[string]any)
	for _, parameters := range []any{pathItem["parameters"], operationSpec["parameters"]} {
		items, _ := parameters.([]any)
		for _, item := range items {
			parameter, _ := item.(map[string]any)
			if parameter["name"] == "perPage" {
				return true
			}
		}
	}
	return false
}

func pf26aCheckError(t *testing.T, response *testutil.Response, status int, code string, schema *jsonschema.Schema) {
	t.Helper()
	testutil.AssertError(t, response, status, code)
	validateJSON(t, schema, response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	for key := range body {
		if key != "code" && key != "message" && key != "errors" {
			t.Errorf("error response has unsupported key %q", key)
		}
	}
	_, hasErrors := body["errors"]
	if hasErrors && status != http.StatusUnprocessableEntity {
		t.Errorf("error response contains errors key for status %d", status)
	}
	if _, hasDocURL := body["doc_url"]; hasDocURL {
		t.Error("error response unexpectedly contains doc_url")
	}
}

func pf26aAssertStatus(t *testing.T, response *testutil.Response, want string) {
	t.Helper()
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode ticket status: %v", err)
	}
	if body.Status != want {
		t.Fatalf("ticket status=%q, want %q (body %s)", body.Status, want, response.Body)
	}
}
