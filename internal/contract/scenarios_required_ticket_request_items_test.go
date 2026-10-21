package contract_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestContractTicketRequiredRequestProperties(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	errorSchema, err := compiler.Compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
	if err != nil {
		t.Fatal(err)
	}
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	baseCreate := map[string]any{"status": "draft", "workflowId": workflowID, "authorTeamId": teamID}
	create := func(appendix map[string]any) map[string]any {
		body := make(map[string]any, len(baseCreate)+len(appendix))
		for key, value := range baseCreate {
			body[key] = value
		}
		for key, value := range appendix {
			body[key] = value
		}
		return body
	}
	const ticketPath = "/v1/tickets/00000000-0000-4000-8000-000000000001"
	cases := []struct {
		name, method, path string
		body               map[string]any
	}{
		{name: "create approver userId missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"approvers": []any{map[string]any{"routeStepCode": "nominee"}}})},
		{name: "create null approver item", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"approvers": []any{nil}})},
		{name: "create approver routeStepCode null", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"approvers": []any{map[string]any{"userId": []string{}, "routeStepCode": nil}}})},
		{name: "update approver routeStepCode missing", method: http.MethodPatch, path: ticketPath, body: map[string]any{"approvers": []any{map[string]any{"userId": []string{}}}}},
		{name: "update approver userId null", method: http.MethodPatch, path: ticketPath, body: map[string]any{"approvers": []any{map[string]any{"userId": nil, "routeStepCode": "nominee"}}}},
		{name: "create inline viewer userId missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"viewers": []any{map[string]any{}}})},
		{name: "update inline viewer userId null", method: http.MethodPatch, path: ticketPath, body: map[string]any{"viewers": []any{map[string]any{"userId": nil}}}},
		{name: "create input identifier alternatives missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"inputs": []any{map[string]any{"value": "x"}}})},
		{name: "update input identifier alternatives null", method: http.MethodPatch, path: ticketPath, body: map[string]any{"inputs": []any{map[string]any{"formFieldCode": nil}}}},
		{name: "form patch input identifier alternatives missing", method: http.MethodPatch, path: ticketPath + "/form", body: map[string]any{"inputs": []any{map[string]any{}}}},
		{name: "create slip row inputs missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"slipItems": []any{map[string]any{}}})},
		{name: "update slip row inputs null", method: http.MethodPatch, path: ticketPath, body: map[string]any{"slipItems": []any{map[string]any{"inputs": nil}}}},
		{name: "create slip field identifier alternatives missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"slipItems": []any{map[string]any{"inputs": []any{map[string]any{}}}}})},
		{name: "update slip field identifier alternatives missing", method: http.MethodPatch, path: ticketPath, body: map[string]any{"slipItems": []any{map[string]any{"inputs": []any{map[string]any{}}}}}},
		{name: "create null slip field item", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"slipItems": []any{map[string]any{"inputs": []any{nil}}}})},
		{name: "form patch slip section identifier alternatives missing", method: http.MethodPatch, path: ticketPath + "/form", body: map[string]any{"slipItems": []any{map[string]any{"inputs": []any{}}}}},
		{name: "form patch slip field identifier alternatives missing", method: http.MethodPatch, path: ticketPath + "/form", body: map[string]any{"slipItems": []any{map[string]any{"slipSectionCode": "lines", "inputs": []any{map[string]any{}}}}}},
		{name: "create CloudSign document id missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"cloudSignDocument": map[string]any{"title": "title", "sandbox": true}})},
		{name: "create CloudSign document title missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "sandbox": true}})},
		{name: "create CloudSign document sandbox missing", method: http.MethodPost, path: "/v1/tickets", body: create(map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "title": "title"}})},
		{name: "update CloudSign document id missing", method: http.MethodPatch, path: ticketPath, body: map[string]any{"cloudSignDocument": map[string]any{"title": "title", "sandbox": true}}},
		{name: "update CloudSign document title missing", method: http.MethodPatch, path: ticketPath, body: map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "sandbox": true}}},
		{name: "update CloudSign document sandbox missing", method: http.MethodPatch, path: ticketPath, body: map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "title": "title"}}},
		{name: "createViewer user and team missing", method: http.MethodPost, path: ticketPath + "/viewers", body: map[string]any{}},
		{name: "createViewer user and team null", method: http.MethodPost, path: ticketPath + "/viewers", body: map[string]any{"userId": nil, "teamId": nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := env.Do(t, testutil.Employee, tc.method, tc.path, tc.body)
			pf26aCheckError(t, response, http.StatusBadRequest, "invalid_parameter", errorSchema)
		})
	}
}
