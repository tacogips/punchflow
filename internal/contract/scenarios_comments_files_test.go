package contract_test

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var liveCommentsFilesOperations = []string{
	"uploadFile", "getFile", "listComments", "createComment", "getComment", "updateComment", "deleteComment",
}

func TestContractLiveCommentsFiles(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	schemas := map[string]*jsonschema.Schema{}
	operations := make(map[string]apiv1.Operation)
	for _, op := range apiv1.Operations() {
		operations[op.OperationID] = op
	}
	passed := make(map[string]bool, len(liveCommentsFilesOperations))
	run := func(operationID string, fn func(*testing.T)) {
		passed[operationID] = t.Run(operationID, fn)
	}
	check := func(t *testing.T, operationID string, response *testutil.Response) {
		t.Helper()
		op, ok := operations[operationID]
		if !ok {
			t.Fatalf("operation %s is not registered", operationID)
		}
		pf26bCheck(t, spec, compiler, schemas, op, response)
	}
	checkError := func(t *testing.T, response *testutil.Response, status int, code string) {
		t.Helper()
		pf26bCheckError(t, compiler, schemas, response, status, code)
	}

	var signedID string
	run("uploadFile", func(t *testing.T) {
		response := pf26bUploadFile(t, env, []byte("contract receipt"), true)
		check(t, "uploadFile", response)
		var uploaded struct {
			SignedID string `json:"signedId"`
		}
		if err := json.Unmarshal(response.Body, &uploaded); err != nil {
			t.Fatal(err)
		}
		if uploaded.SignedID == "" {
			t.Fatal("upload returned an empty signedId")
		}
		signedID = uploaded.SignedID

		t.Run("error_invalid_parameter", func(t *testing.T) {
			missing := pf26bUploadFile(t, env, nil, false)
			checkError(t, missing, http.StatusBadRequest, "invalid_parameter")
		})
	})

	ticketID := pf26bCreateExpenseTicket(t, env, signedID)
	var commentID string
	run("getFile", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/files/"+signedID, nil)
		check(t, "getFile", response)
		t.Run("error_resource_not_found", func(t *testing.T) {
			unattached := pf26bUploadFile(t, env, []byte("unattached"), true)
			var uploaded struct {
				SignedID string `json:"signedId"`
			}
			if err := json.Unmarshal(unattached.Body, &uploaded); err != nil {
				t.Fatal(err)
			}
			if uploaded.SignedID == "" {
				t.Fatal("unattached upload returned an empty signedId")
			}
			missing := env.Do(t, testutil.Employee, http.MethodGet, "/v1/files/"+uploaded.SignedID, nil)
			checkError(t, missing, http.StatusNotFound, "resource_not_found")
		})
	})

	commentPath := "/v1/tickets/" + ticketID + "/comments"
	run("createComment", func(t *testing.T) {
		created := env.Do(t, testutil.Employee, http.MethodPost, commentPath, map[string]any{"body": "contract comment"})
		check(t, "createComment", created)
		var comment struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(created.Body, &comment); err != nil {
			t.Fatal(err)
		}
		if comment.ID == "" {
			t.Fatal("createComment returned an empty id")
		}
		commentID = comment.ID
		second := env.Do(t, testutil.Employee, http.MethodPost, commentPath, map[string]any{"body": "second contract comment"})
		check(t, "createComment", second)
	})

	run("getComment", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodGet, commentPath+"/"+commentID, nil)
		check(t, "getComment", response)
		t.Run("error_resource_not_found", func(t *testing.T) {
			missing := env.Do(t, testutil.Employee, http.MethodGet, commentPath+"/00000000-0000-4000-8000-000000000000", nil)
			checkError(t, missing, http.StatusNotFound, "resource_not_found")
		})
	})

	run("updateComment", func(t *testing.T) {
		updated := env.Do(t, testutil.Employee, http.MethodPatch, commentPath+"/"+commentID, map[string]any{"body": "updated contract comment"})
		check(t, "updateComment", updated)
		t.Run("error_missing_permission", func(t *testing.T) {
			denied := env.Do(t, testutil.Manager, http.MethodPatch, commentPath+"/"+commentID, map[string]any{"body": "not the author"})
			checkError(t, denied, http.StatusForbidden, "missing_permission")
		})
	})

	run("listComments", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodGet, commentPath+"?perPage=1", nil)
		check(t, "listComments", response)
		if !strings.Contains(response.Header.Get("Link"), `rel="next"`) {
			t.Errorf("Link header %q does not advertise rel=next", response.Header.Get("Link"))
		}
	})

	run("deleteComment", func(t *testing.T) {
		deleted := env.Do(t, testutil.Employee, http.MethodDelete, commentPath+"/"+commentID, nil)
		check(t, "deleteComment", deleted)
		followup := env.Do(t, testutil.Employee, http.MethodGet, commentPath+"/"+commentID, nil)
		check(t, "getComment", followup)
		var comment map[string]any
		if err := json.Unmarshal(followup.Body, &comment); err != nil {
			t.Fatal(err)
		}
		deletedAt, ok := comment["deletedAt"].(string)
		if !ok || deletedAt == "" {
			t.Fatalf("deleted comment deletedAt=%v, want timestamp", comment["deletedAt"])
		}
	})

	for _, operationID := range liveCommentsFilesOperations {
		if _, ok := operations[operationID]; !ok {
			t.Errorf("inventory operation %s is not registered", operationID)
		}
		if !passed[operationID] {
			t.Errorf("inventory operation %s has no passing subtest", operationID)
		}
	}
}

func pf26bCheck(t *testing.T, spec map[string]any, compiler *jsonschema.Compiler, cache map[string]*jsonschema.Schema, op apiv1.Operation, response *testutil.Response) {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", op.OperationID, response.Status, response.Body)
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
		t.Fatalf("%s has no 200 response", op.OperationID)
	}
	content, _ := responseSpec["content"].(map[string]any)
	if len(content) == 0 {
		if len(response.Body) != 0 {
			t.Fatalf("%s returned %d bytes, want an empty response", op.OperationID, len(response.Body))
		}
	} else {
		jsonContent, ok := content["application/json"].(map[string]any)
		if !ok {
			t.Fatalf("%s 200 response has no application/json schema", op.OperationID)
		}
		if _, ok := jsonContent["schema"]; !ok {
			t.Fatalf("%s 200 response schema is missing", op.OperationID)
		}
		uri := responseSchemaURI(op.Pattern, strings.ToLower(op.Method))
		schema := cache[uri]
		if schema == nil {
			var err error
			schema, err = compiler.Compile(uri)
			if err != nil {
				t.Fatalf("compile %s schema %s: %v", op.OperationID, uri, err)
			}
			cache[uri] = schema
		}
		validateJSON(t, schema, response.Body)
		var decoded any
		if err := json.Unmarshal(response.Body, &decoded); err != nil {
			t.Fatalf("decode %s response: %v", op.OperationID, err)
		}
		checkTimestamps(t, decoded)
	}
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["name"] != "perPage" {
			continue
		}
		for _, header := range []string{"Page", "Per-Page", "Total"} {
			if response.Header.Get(header) == "" {
				t.Errorf("%s response missing %s pagination header", op.OperationID, header)
			}
		}
		break
	}
}

func pf26bCheckError(t *testing.T, compiler *jsonschema.Compiler, cache map[string]*jsonschema.Schema, response *testutil.Response, status int, code string) {
	t.Helper()
	testutil.AssertError(t, response, status, code)
	const uri = "file:///kickflow-openapi.json#/components/schemas/ErrorResponse"
	schema := cache[uri]
	if schema == nil {
		var err error
		schema, err = compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile ErrorResponse schema: %v", err)
		}
		cache[uri] = schema
	}
	validateJSON(t, schema, response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	for key := range body {
		if key != "code" && key != "message" && key != "errors" {
			t.Errorf("error response contains unexpected key %q", key)
		}
	}
	_, hasErrors := body["errors"]
	if hasErrors != (status == http.StatusUnprocessableEntity) {
		t.Errorf("error response errors present=%t for status %d", hasErrors, status)
	}
	if _, exists := body["doc_url"]; exists {
		t.Error("error response unexpectedly contains doc_url")
	}
}

func pf26bUploadFile(t *testing.T, env *testutil.Env, data []byte, includeFile bool) *testutil.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeFile {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": "receipt.pdf"}))
		header.Set("Content-Type", "application/pdf")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.URL+"/v1/files", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer pf_pat_dev_employee")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return env.DoRaw(t, request)
}

func pf26bCreateExpenseTicket(t *testing.T, env *testutil.Env, signedID string) string {
	t.Helper()
	workflow := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if workflow.Status != http.StatusOK {
		t.Fatalf("get expense workflow: status=%d body=%s", workflow.Status, workflow.Body)
	}
	var workflowBody map[string]any
	workflow.JSON(t, &workflowBody)
	fieldIDs := map[string]string{}
	fieldCodes := []string{}
	sections, _ := workflowBody["sectionList"].([]any)
	for _, rawSection := range sections {
		section, _ := rawSection.(map[string]any)
		fields, _ := section["formFields"].([]any)
		for _, rawField := range fields {
			field, _ := rawField.(map[string]any)
			code, _ := field["code"].(string)
			id, _ := field["id"].(string)
			if code != "" && id != "" {
				fieldIDs[code] = id
				fieldCodes = append(fieldCodes, code)
			}
		}
	}
	for _, code := range []string{"amount", "purpose", "receipt"} {
		if fieldIDs[code] == "" {
			t.Fatalf("expense workflow field %q not found", code)
		}
	}
	chart := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationChart", nil)
	if chart.Status != http.StatusOK {
		t.Fatalf("get current organization chart: status=%d body=%s", chart.Status, chart.Body)
	}
	var chartBody map[string]any
	chart.JSON(t, &chartBody)
	chartID, _ := chartBody["id"].(string)
	if chartID == "" {
		t.Fatal("current organization chart returned an empty id")
	}
	team := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+chartID+"/teams/sales", nil)
	if team.Status != http.StatusOK {
		t.Fatalf("get sales team: status=%d body=%s", team.Status, team.Body)
	}
	var teamBody map[string]any
	team.JSON(t, &teamBody)
	teamID, _ := teamBody["id"].(string)
	if teamID == "" {
		t.Fatal("sales team returned an empty id")
	}
	inputs := make([]map[string]any, 0, len(fieldCodes))
	for _, code := range fieldCodes {
		input := map[string]any{"formFieldId": fieldIDs[code], "value": nil}
		switch code {
		case "amount":
			input["value"] = "1000"
		case "purpose":
			input["value"] = "Contract scenario"
		case "receipt":
			delete(input, "value")
			input["files"] = []string{signedID}
		}
		inputs = append(inputs, input)
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": "in_progress", "workflowId": workflowBody["id"], "authorTeamId": teamID,
		"title":  "PF-26b contract comments and files",
		"inputs": inputs,
	})
	if created.Status != http.StatusOK {
		t.Fatalf("create expense ticket: status=%d body=%s", created.Status, created.Body)
	}
	var ticket struct {
		ID string `json:"id"`
	}
	created.JSON(t, &ticket)
	if ticket.ID == "" {
		t.Fatal("create ticket returned an empty id")
	}
	return ticket.ID
}
