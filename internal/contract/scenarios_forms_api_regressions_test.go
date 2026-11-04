package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const pf27CheckboxFieldID = "00000000-0000-4000-8000-0000000000b1"

func TestContractTicketFormRequestValidation(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	errorSchema, err := compiler.Compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
	if err != nil {
		t.Fatal(err)
	}
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}

	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	pf27AddCheckboxField(t, env, workflowID)
	fields := pf26aFormFields(t, env)
	salesTeamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	masterItemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatal(err)
	}
	validInputs := []map[string]any{
		{"formFieldId": fields["amount"], "value": "5000"},
		{"formFieldId": fields["purpose"], "value": "Contract validation"},
		{"formFieldId": fields["expense-date"], "value": "2026-01-01"},
		{"formFieldId": fields["cost-center"], "generalMasterItemId": masterItemID},
		{"formFieldId": fields["receipt"], "files": []string{}},
		{"formFieldId": fields["approver-note"], "value": ""},
		{"formFieldId": fields["wire-checkbox"], "value": []string{}},
	}
	createBody := func(inputs []map[string]any) map[string]any {
		return map[string]any{"status": "draft", "workflowId": workflowID, "authorTeamId": salesTeamID, "inputs": inputs}
	}

	t.Run("create_rejects_numeric_value", func(t *testing.T) {
		inputs := append([]map[string]any(nil), validInputs...)
		inputs[0] = map[string]any{"formFieldId": fields["amount"], "value": 5000}
		response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", createBody(inputs))
		pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
	})
	t.Run("create_underflow_numeric_has_bounded_latency", func(t *testing.T) {
		inputs := append([]map[string]any(nil), validInputs...)
		inputs[0] = map[string]any{"formFieldId": fields["amount"], "value": "1e-400000000"}
		start := time.Now()
		response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", createBody(inputs))
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Fatalf("extreme underflow request took %s", elapsed)
		}
		pf26aCheck(t, operations["createTicket"], response, pf16ResponseSchema(t, compiler, operations["createTicket"]), spec)
	})
	t.Run("create_rejects_scalar_checkbox", func(t *testing.T) {
		inputs := append([]map[string]any(nil), validInputs...)
		inputs[len(inputs)-1] = map[string]any{"formFieldId": fields["wire-checkbox"], "value": "selected"}
		response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", createBody(inputs))
		pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
	})

	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", createBody(validInputs))
	if created.Status != http.StatusOK {
		t.Fatalf("create valid ticket: status=%d body=%s", created.Status, created.Body)
	}
	var ticket pf26aTicket
	created.JSON(t, &ticket)
	t.Run("full_update_rejects_numeric_value", func(t *testing.T) {
		inputs := []map[string]any{{"formFieldId": fields["amount"], "value": 6000}}
		response := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID, map[string]any{"inputs": inputs})
		pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
	})
	t.Run("differential_update_keeps_broader_value_shape", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID+"/form", map[string]any{
			"inputs": []map[string]any{{"formFieldId": fields["amount"], "value": 6000}},
		})
		pf26aCheck(t, operations["updateTicketForm"], response, pf16ResponseSchema(t, compiler, operations["updateTicketForm"]), spec)
	})
	t.Run("differential_update_rejects_scalar_checkbox", func(t *testing.T) {
		response := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID+"/form", map[string]any{
			"inputs": []map[string]any{{"formFieldId": fields["wire-checkbox"], "value": "selected"}},
		})
		pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
	})
}

func TestContractFullApproverUpdateRequiresCompleteInputs(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	errorSchema, err := compiler.Compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
	if err != nil {
		t.Fatal(err)
	}
	operations := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	createSchema, err := compiler.Compile(responseSchemaURI(operations["createTicket"].Pattern, "post"))
	if err != nil {
		t.Fatal(err)
	}
	ticket := pf26aCreateTicket(t, env, operations["createTicket"], map[string]*jsonschema.Schema{"createTicket": createSchema}, spec, "in_progress")
	response := env.Do(t, testutil.Manager, http.MethodPatch, "/v1/tickets/"+ticket.ID, map[string]any{
		"inputs": []map[string]any{{"formFieldCode": "amount", "value": "6000"}},
	})
	pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
}

func pf27AddCheckboxField(t *testing.T, env *testutil.Env, workflowID string) {
	t.Helper()
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	field := model.FormField{
		ID: pf27CheckboxFieldID, Code: "wire-checkbox", Title: "Wire checkbox", FieldType: "checkbox",
		Author: true, Approver: true, Size: "full", Orientation: "horizontal", Options: []string{"selected"},
	}
	for i := range def.SectionList {
		if def.SectionList[i].SectionType == model.SectionTypeForm {
			def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, field)
			break
		}
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID); err != nil {
		t.Fatal(err)
	}
}

func TestContractRequiredMasterCheckboxRejectsEmptySelection(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	cache := &pf26eSchemaCache{compiler: schemaCompiler(t, spec), compiled: make(map[string]*jsonschema.Schema)}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{
		"code": "pf27-checkbox-master", "name": "Required checkbox master",
		"fields": []any{map[string]any{"code": "required-check", "title": "Required check", "required": true, "fieldType": "checkbox", "options": []string{"yes", "no"}}},
	})
	pf26eCheck(t, "createGeneralMaster", created, spec, cache)
	response := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/pf27-checkbox-master/items", map[string]any{
		"code": "pf27-empty-checkbox", "name": "Empty required checkbox",
		"inputs": []any{map[string]any{"code": "required-check", "value": []string{}}},
	})
	pf26eCheckError(t, response, http.StatusUnprocessableEntity, "validation_failed", cache)
}
