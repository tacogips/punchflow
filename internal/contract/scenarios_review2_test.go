package contract_test

import (
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	errorResponseURI = "file:///kickflow-openapi.json#/components/schemas/ErrorResponse"
	missingUserID    = "00000000-0000-4000-8000-00000000ffff"
)

func requestSchemaURI(path, method string) string {
	return "file:///kickflow-openapi.json#/paths/" + escapePointer(path) + "/" + method + "/requestBody/content/application~1json/schema"
}

// N03: createTicket and updateTicket accept viewers[].userId and watcherUserId
// (the snapshot marks them as changelog additions), and unknown or deleted users
// are a 422 validation_failed ErrorResponse.
func TestContractTicketInlineViewersAndWatcher(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	compile := func(uri string) *jsonschema.Schema {
		t.Helper()
		schema, err := compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile %s: %v", uri, err)
		}
		return schema
	}
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	createRequest := compile(requestSchemaURI(operations["createTicket"].Pattern, "post"))
	updateRequest := compile(requestSchemaURI(operations["updateTicket"].Pattern, "patch"))
	errorSchema := compile(errorResponseURI)
	detailSchema := compile(specPrefix + "TicketDetail")

	manager := env.UserID(t, "sales-manager")
	finance := env.UserID(t, "finance-manager")
	base := n03Body(t, env)

	valid := map[string]any{"viewers": []map[string]any{{"userId": manager}}, "watcherUserId": finance}
	body := map[string]any{}
	for key, value := range base {
		body[key] = value
	}
	for key, value := range valid {
		body[key] = value
	}
	validateAny(t, createRequest, body)
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", body)
	if created.Status != http.StatusOK {
		t.Fatalf("create with viewers: %d %s", created.Status, created.Body)
	}
	validateJSON(t, detailSchema, created.Body)
	var ticket struct {
		ID string `json:"id"`
	}
	created.JSON(t, &ticket)

	patch := map[string]any{"viewers": []map[string]any{{"userId": finance}}, "watcherUserId": manager}
	validateAny(t, updateRequest, patch)
	updated := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID, patch)
	if updated.Status != http.StatusOK {
		t.Fatalf("update with viewers: %d %s", updated.Status, updated.Body)
	}
	validateJSON(t, detailSchema, updated.Body)

	for _, invalid := range []map[string]any{
		{"viewers": []map[string]any{{"userId": missingUserID}}},
		{"watcherUserId": missingUserID},
	} {
		response := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID, invalid)
		testutil.AssertError(t, response, http.StatusUnprocessableEntity, "validation_failed")
		validateJSON(t, errorSchema, response.Body)
	}

	// Deleted (deactivated) users are rejected on creation, suspended users are not.
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE users SET status='deactivated', deactivated_at=1000 WHERE id=?`, finance); err != nil {
		t.Fatal(err)
	}
	rejected := map[string]any{}
	for key, value := range base {
		rejected[key] = value
	}
	rejected["watcherUserId"] = finance
	response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", rejected)
	testutil.AssertError(t, response, http.StatusUnprocessableEntity, "validation_failed")
	validateJSON(t, errorSchema, response.Body)
}

// N08: the user email is an address-only email in requests and responses.
func TestContractUserEmailIsAddressOnly(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	userSchema, err := compiler.Compile(specPrefix + "User")
	if err != nil {
		t.Fatal(err)
	}
	errorSchema, err := compiler.Compile(errorResponseURI)
	if err != nil {
		t.Fatal(err)
	}
	mailbox := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "Display Name <mailbox@example.test>", "firstName": "Mail", "lastName": "Box"})
	testutil.AssertError(t, mailbox, http.StatusBadRequest, "invalid_parameter")
	validateJSON(t, errorSchema, mailbox.Body)

	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "address-only@example.test", "firstName": "Address", "lastName": "Only"})
	if created.Status != http.StatusOK {
		t.Fatalf("create user: %d %s", created.Status, created.Body)
	}
	validateJSON(t, userSchema, created.Body)
	var user struct {
		ID string `json:"id"`
	}
	created.JSON(t, &user)

	renamed := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+user.ID, map[string]any{"email": "Other Name <other@example.test>"})
	testutil.AssertError(t, renamed, http.StatusBadRequest, "invalid_parameter")
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+user.ID, map[string]any{"email": "other@example.test"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update user: %d %s", updated.Status, updated.Body)
	}
	validateJSON(t, userSchema, updated.Body)
}

// NF-15: commenting on a workflow with comments disabled is a 422
// validation_failed ErrorResponse carrying errors.base.
func TestContractCommentsDisabledIsValidationFailed(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	errorSchema, err := compiler.Compile(errorResponseURI)
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	ticketID := n03Body(t, env)
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", ticketID)
	if created.Status != http.StatusOK {
		t.Fatalf("create: %d %s", created.Status, created.Body)
	}
	var ticket struct {
		ID string `json:"id"`
	}
	created.JSON(t, &ticket)
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=json_set(definition_json,'$.commentingEnabled',json('false')) WHERE id=?`, workflowID); err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticket.ID+"/comments", map[string]any{"body": "nope"})
	testutil.AssertError(t, response, http.StatusUnprocessableEntity, "validation_failed")
	validateJSON(t, errorSchema, response.Body)
}
