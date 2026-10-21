package contract_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var liveMastersOperations = []string{
	"createCategory", "deleteCategory", "updateCategory",
	"createFolder", "deleteFolder", "updateFolder",
	"createGeneralMaster", "updateGeneralMaster", "deleteGeneralMaster",
	"listGeneralMasterItems", "createGeneralMasterItem", "getGeneralMasterItem",
	"updateGeneralMasterItem", "deleteGeneralMasterItem",
}

type pf26eSchemaCache struct {
	compiler *jsonschema.Compiler
	compiled map[string]*jsonschema.Schema
}

func pf26eCompile(t *testing.T, cache *pf26eSchemaCache, uri string) *jsonschema.Schema {
	t.Helper()
	if schema, ok := cache.compiled[uri]; ok {
		return schema
	}
	schema, err := cache.compiler.Compile(uri)
	if err != nil {
		t.Fatalf("compile schema %s: %v", uri, err)
	}
	cache.compiled[uri] = schema
	return schema
}

func pf26eCheck(t *testing.T, operationID string, response *testutil.Response, spec map[string]any, cache *pf26eSchemaCache) {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", operationID, response.Status, response.Body)
	}
	operations := spec["paths"].(map[string]any)
	var operation map[string]any
	var operationPath, operationMethod string
	for _, registered := range apiv1.Operations() {
		if registered.OperationID == operationID {
			pathItem := operations[registered.Pattern].(map[string]any)
			operation = pathItem[strings.ToLower(registered.Method)].(map[string]any)
			operationPath = registered.Pattern
			operationMethod = strings.ToLower(registered.Method)
			break
		}
	}
	if operation == nil {
		t.Fatalf("%s is not registered in the OpenAPI paths", operationID)
	}
	responses := operation["responses"].(map[string]any)
	responseSpec, ok := responses["200"].(map[string]any)
	if !ok {
		responseSpec, ok = responses["200.0"].(map[string]any)
	}
	if !ok {
		t.Fatalf("%s has no 200 response schema", operationID)
	}
	content, _ := responseSpec["content"].(map[string]any)
	if content == nil {
		if len(response.Body) != 0 {
			t.Fatalf("%s returned a body for an empty 200 response: %s", operationID, response.Body)
		}
		return
	}
	uri := responseSchemaURI(operationPath, operationMethod)
	validateJSON(t, pf26eCompile(t, cache, uri), response.Body)
	var decoded any
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		t.Fatalf("decode %s response: %v", operationID, err)
	}
	checkTimestamps(t, decoded)
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["name"] == "perPage" {
			for _, header := range []string{"Page", "Per-Page", "Total"} {
				if response.Header.Get(header) == "" {
					t.Errorf("%s missing %s pagination header", operationID, header)
				}
			}
		}
	}
}

func pf26eCheckError(t *testing.T, response *testutil.Response, status int, code string, cache *pf26eSchemaCache) {
	t.Helper()
	testutil.AssertError(t, response, status, code)
	validateJSON(t, pf26eCompile(t, cache, "file:///kickflow-openapi.json#/components/schemas/ErrorResponse"), response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	for key := range body {
		if key != "code" && key != "message" && key != "errors" {
			t.Errorf("error response has undocumented key %q", key)
		}
	}
	_, hasErrors := body["errors"]
	if hasErrors != (status == http.StatusUnprocessableEntity) {
		t.Errorf("errors presence=%t for status %d; want %t", hasErrors, status, status == http.StatusUnprocessableEntity)
	}
	if _, hasDocURL := body["doc_url"]; hasDocURL {
		t.Error("error response contains doc_url")
	}
}

func TestContractLiveMasters(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	cache := &pf26eSchemaCache{compiler: schemaCompiler(t, spec), compiled: make(map[string]*jsonschema.Schema)}
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	passed := make(map[string]bool, len(liveMastersOperations))
	run := func(operationID string, action func(*testing.T)) {
		passed[operationID] = t.Run(operationID, func(t *testing.T) {
			if _, ok := operations[operationID]; !ok {
				t.Fatalf("operation %s is not registered", operationID)
			}
			action(t)
		})
	}

	category := ""
	run("createCategory", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/categories", map[string]any{"name": "PF26e temporary category"})
		pf26eCheck(t, "createCategory", response, spec, cache)
		var body map[string]any
		response.JSON(t, &body)
		category = body["id"].(string)
		t.Run("error_missing_permission", func(t *testing.T) {
			denied := env.Do(t, testutil.Employee, http.MethodPost, "/v1/categories", map[string]any{"name": "Denied category"})
			pf26eCheckError(t, denied, http.StatusForbidden, "missing_permission", cache)
		})
	})
	run("updateCategory", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/categories/"+category, map[string]any{"name": "PF26e updated category"})
		pf26eCheck(t, "updateCategory", response, spec, cache)
	})
	run("deleteCategory", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/categories/"+category, nil)
		pf26eCheck(t, "deleteCategory", response, spec, cache)
		t.Run("error_resource_not_found", func(t *testing.T) {
			missing := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/categories/"+category, nil)
			pf26eCheckError(t, missing, http.StatusNotFound, "resource_not_found", cache)
		})
	})

	rootResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/folders/root", nil)
	if rootResponse.Status != http.StatusOK {
		t.Fatalf("GET /v1/folders/root status=%d body=%s", rootResponse.Status, rootResponse.Body)
	}
	pf26eCheck(t, "getFolder", rootResponse, spec, cache)
	var root map[string]any
	rootResponse.JSON(t, &root)
	folder := "pf26e-folder"
	run("createFolder", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{
			"name": "PF26e temporary folder", "code": folder, "parentFolderId": root["id"],
		})
		pf26eCheck(t, "createFolder", response, spec, cache)
	})
	run("updateFolder", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/folders/"+folder, map[string]any{"name": "PF26e updated folder"})
		pf26eCheck(t, "updateFolder", response, spec, cache)
		t.Run("error_resource_not_editable", func(t *testing.T) {
			denied := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/folders/root", map[string]any{"name": "Changed root"})
			pf26eCheckError(t, denied, http.StatusForbidden, "resource_not_editable", cache)
		})
	})
	run("deleteFolder", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/"+folder, nil)
		pf26eCheck(t, "deleteFolder", response, spec, cache)
	})

	master := "pf26e-master"
	run("createGeneralMaster", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{
			"code": master, "name": "PF26e temporary master",
			"fields": []any{
				map[string]any{"code": "note", "title": "Note", "required": false, "fieldType": "text"},
				map[string]any{"code": "optional", "title": "Optional", "required": false, "fieldType": "text"},
			},
		})
		pf26eCheck(t, "createGeneralMaster", response, spec, cache)
		t.Run("error_missing_permission", func(t *testing.T) {
			denied := env.Do(t, testutil.Employee, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Denied master"})
			pf26eCheckError(t, denied, http.StatusForbidden, "missing_permission", cache)
		})
	})
	run("updateGeneralMaster", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/"+master, map[string]any{"name": "PF26e updated master"})
		pf26eCheck(t, "updateGeneralMaster", response, spec, cache)
	})
	item := "pf26e-item"
	secondItem := "pf26e-item2"
	itemInputID := ""
	run("createGeneralMasterItem", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+master+"/items", map[string]any{
			"code": item, "name": "PF26e temporary item", "inputs": []any{
				map[string]any{"code": "note", "value": "first"},
				map[string]any{"code": "optional", "value": nil},
			},
		})
		pf26eCheck(t, "createGeneralMasterItem", response, spec, cache)
		var createdItem struct {
			Inputs []struct {
				ID string `json:"id"`
			} `json:"inputs"`
		}
		response.JSON(t, &createdItem)
		if len(createdItem.Inputs) != 2 {
			t.Fatalf("created general master item has %d inputs, want 2", len(createdItem.Inputs))
		}
		itemInputID = createdItem.Inputs[0].ID
		t.Run("error_omitted_optional_input", func(t *testing.T) {
			incomplete := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+master+"/items", map[string]any{
				"code": "pf26e-incomplete", "name": "Incomplete item",
				"inputs": []any{map[string]any{"code": "note", "value": "only one"}},
			})
			pf26eCheckError(t, incomplete, http.StatusUnprocessableEntity, "validation_failed", cache)
		})
		t.Run("error_empty_inputs", func(t *testing.T) {
			incomplete := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+master+"/items", map[string]any{
				"code": "pf26e-empty", "name": "Empty item", "inputs": []any{},
			})
			pf26eCheckError(t, incomplete, http.StatusUnprocessableEntity, "validation_failed", cache)
		})
		second := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+master+"/items", map[string]any{
			"code": secondItem, "name": "PF26e second item", "inputs": []any{
				map[string]any{"code": "note", "value": "second"},
				map[string]any{"code": "optional", "value": "extra"},
			},
		})
		pf26eCheck(t, "createGeneralMasterItem", second, spec, cache)
	})
	run("getGeneralMasterItem", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/"+master+"/items/"+item, nil)
		pf26eCheck(t, "getGeneralMasterItem", response, spec, cache)
		t.Run("error_resource_not_found", func(t *testing.T) {
			missing := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/"+master+"/items/no-such-item", nil)
			pf26eCheckError(t, missing, http.StatusNotFound, "resource_not_found", cache)
		})
	})
	run("listGeneralMasterItems", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/"+master+"/items", nil)
		pf26eCheck(t, "listGeneralMasterItems", response, spec, cache)
		page := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/"+master+"/items?perPage=1", nil)
		pf26eCheck(t, "listGeneralMasterItems", page, spec, cache)
		if !strings.Contains(page.Header.Get("Link"), `rel="next"`) {
			t.Errorf("Link header %q does not contain rel=next", page.Header.Get("Link"))
		}
	})
	run("updateGeneralMasterItem", func(t *testing.T) {
		incomplete := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/"+master+"/items/"+item, map[string]any{
			"inputs": []any{map[string]any{"code": "note", "value": "partial"}},
		})
		pf26eCheckError(t, incomplete, http.StatusUnprocessableEntity, "validation_failed", cache)
		response := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/"+master+"/items/"+item, map[string]any{
			"name": "PF26e updated item", "inputs": []any{
				map[string]any{"code": "note", "value": "updated"},
				map[string]any{"code": "optional", "value": nil},
			},
		})
		pf26eCheck(t, "updateGeneralMasterItem", response, spec, cache)
		var updatedItem struct {
			Inputs []struct {
				ID string `json:"id"`
			} `json:"inputs"`
		}
		response.JSON(t, &updatedItem)
		if len(updatedItem.Inputs) == 0 || updatedItem.Inputs[0].ID != itemInputID {
			t.Errorf("updated general master item input id=%v, want stable id %q", updatedItem.Inputs, itemInputID)
		}
		beforeOmittedPatch := response.Body
		omitted := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/"+master+"/items/"+item, map[string]any{"name": "PF26e patched without inputs"})
		pf26eCheck(t, "updateGeneralMasterItem", omitted, spec, cache)
		var before, after struct {
			Inputs []struct {
				ID    string `json:"id"`
				Value any    `json:"value"`
				Field struct {
					Code string `json:"code"`
				} `json:"field"`
			} `json:"inputs"`
		}
		if err := json.Unmarshal(beforeOmittedPatch, &before); err != nil {
			t.Fatalf("decode item before omitted-input PATCH: %v", err)
		}
		omitted.JSON(t, &after)
		if len(after.Inputs) != len(before.Inputs) {
			t.Fatalf("omitted-input PATCH changed input count from %d to %d", len(before.Inputs), len(after.Inputs))
		}
		for i := range before.Inputs {
			if after.Inputs[i].ID != before.Inputs[i].ID || after.Inputs[i].Field.Code != before.Inputs[i].Field.Code || after.Inputs[i].Value != before.Inputs[i].Value {
				t.Errorf("omitted-input PATCH changed input %d: before=%+v after=%+v", i, before.Inputs[i], after.Inputs[i])
			}
		}
	})
	run("deleteGeneralMasterItem", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/generalMasters/"+master+"/items/"+secondItem, nil)
		pf26eCheck(t, "deleteGeneralMasterItem", response, spec, cache)
	})
	run("deleteGeneralMaster", func(t *testing.T) {
		response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/generalMasters/"+master, nil)
		pf26eCheck(t, "deleteGeneralMaster", response, spec, cache)
	})

	if len(liveMastersOperations) != 14 {
		t.Errorf("live master inventory has %d operations, want 14", len(liveMastersOperations))
	}
	for _, operationID := range liveMastersOperations {
		if _, ok := operations[operationID]; !ok {
			t.Errorf("live master operation %s is not registered", operationID)
		}
		if !passed[operationID] {
			t.Errorf("live master operation %s has no passing subtest", operationID)
		}
	}
}
