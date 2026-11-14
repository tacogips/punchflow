package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestTicketBodiesRejectMissingRequiredNestedProperties(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"status": "draft", "workflowId": workflowID, "authorTeamId": teamID}
	clone := func(values map[string]any) map[string]any {
		body := make(map[string]any, len(base)+len(values))
		for key, value := range base {
			body[key] = value
		}
		for key, value := range values {
			body[key] = value
		}
		return body
	}
	updatePath := "/v1/tickets/00000000-0000-4000-8000-000000000001"
	tests := []struct {
		name, method, path string
		body               map[string]any
	}{
		{name: "create approver missing userId", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"approvers": []any{map[string]any{"routeStepCode": "nominee"}}})},
		{name: "create approver null userId", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"approvers": []any{map[string]any{"routeStepCode": "nominee", "userId": nil}}})},
		{name: "create null approver item", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"approvers": []any{nil}})},
		{name: "create approver missing routeStepCode", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"approvers": []any{map[string]any{"userId": []string{env.UserID(t, "finance-manager")}}}})},
		{name: "create approver null routeStepCode", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"approvers": []any{map[string]any{"userId": []string{env.UserID(t, "finance-manager")}, "routeStepCode": nil}}})},
		{name: "update approver missing userId", method: http.MethodPatch, path: updatePath, body: map[string]any{"approvers": []any{map[string]any{"routeStepCode": "nominee"}}}},
		{name: "update approver null userId", method: http.MethodPatch, path: updatePath, body: map[string]any{"approvers": []any{map[string]any{"routeStepCode": "nominee", "userId": nil}}}},
		{name: "update approver missing routeStepCode", method: http.MethodPatch, path: updatePath, body: map[string]any{"approvers": []any{map[string]any{"userId": []string{env.UserID(t, "finance-manager")}}}}},
		{name: "update approver null routeStepCode", method: http.MethodPatch, path: updatePath, body: map[string]any{"approvers": []any{map[string]any{"userId": []string{env.UserID(t, "finance-manager")}, "routeStepCode": nil}}}},
		{name: "create viewer missing userId", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"viewers": []any{map[string]any{"extra": true}}})},
		{name: "create viewer null userId", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"viewers": []any{map[string]any{"userId": nil}}})},
		{name: "update viewer missing userId", method: http.MethodPatch, path: updatePath, body: map[string]any{"viewers": []any{map[string]any{}}}},
		{name: "update viewer null userId", method: http.MethodPatch, path: updatePath, body: map[string]any{"viewers": []any{map[string]any{"userId": nil}}}},
		{name: "form input missing field alternative", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"inputs": []any{map[string]any{"value": "x"}}})},
		{name: "update input missing field alternative", method: http.MethodPatch, path: updatePath, body: map[string]any{"inputs": []any{map[string]any{"value": "x"}}}},
		{name: "form patch input missing field alternative", method: http.MethodPatch, path: updatePath + "/form", body: map[string]any{"inputs": []any{map[string]any{"value": "x"}}}},
		{name: "create slip item missing inputs", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"slipItems": []any{map[string]any{}}})},
		{name: "update slip item null inputs", method: http.MethodPatch, path: updatePath, body: map[string]any{"slipItems": []any{map[string]any{"inputs": nil}}}},
		{name: "create slip field null item", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"slipItems": []any{map[string]any{"inputs": []any{nil}}}})},
		{name: "form patch slip section missing identifier", method: http.MethodPatch, path: updatePath + "/form", body: map[string]any{"slipItems": []any{map[string]any{"inputs": []any{}}}}},
		{name: "form patch slip row missing inputs", method: http.MethodPatch, path: updatePath + "/form", body: map[string]any{"slipItems": []any{map[string]any{"slipSectionCode": "lines"}}}},
		{name: "create slip field missing identifier alternative", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"slipItems": []any{map[string]any{"inputs": []any{map[string]any{"value": "x"}}}}})},
		{name: "update slip field missing identifier alternative", method: http.MethodPatch, path: updatePath, body: map[string]any{"slipItems": []any{map[string]any{"inputs": []any{map[string]any{"value": "x"}}}}}},
		{name: "form patch slip field missing identifier alternative", method: http.MethodPatch, path: updatePath + "/form", body: map[string]any{"slipItems": []any{map[string]any{"slipSectionCode": "lines", "inputs": []any{map[string]any{"value": "x"}}}}}},
		{name: "create CloudSign document missing id", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"cloudSignDocument": map[string]any{"title": "title", "sandbox": true}})},
		{name: "create CloudSign document missing title", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "sandbox": true}})},
		{name: "create CloudSign document missing sandbox", method: http.MethodPost, path: "/v1/tickets", body: clone(map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "title": "title"}})},
		{name: "update CloudSign document missing id", method: http.MethodPatch, path: updatePath, body: map[string]any{"cloudSignDocument": map[string]any{"title": "title", "sandbox": true}}},
		{name: "update CloudSign document missing title", method: http.MethodPatch, path: updatePath, body: map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "sandbox": true}}},
		{name: "update CloudSign document missing sandbox", method: http.MethodPatch, path: updatePath, body: map[string]any{"cloudSignDocument": map[string]any{"id": "doc", "title": "title"}}},
		{name: "create viewer missing user or team", method: http.MethodPost, path: updatePath + "/viewers", body: map[string]any{}},
		{name: "create viewer null user and team", method: http.MethodPost, path: updatePath + "/viewers", body: map[string]any{"userId": nil, "teamId": nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := env.Do(t, testutil.Employee, tc.method, tc.path, tc.body)
			testutil.AssertError(t, got, http.StatusBadRequest, "invalid_parameter")
		})
	}
}

func TestTicketRequiredNestedValidationKeepsUnknownFieldsIgnored(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": "draft", "workflowId": workflowID, "authorTeamId": teamID,
		"futureProperty": "ignored", "inputs": []any{
			map[string]any{"formFieldId": "6f32e53a-d4ce-4d19-8723-754749c7d803", "value": "100", "futureInputProperty": true},
			map[string]any{"formFieldId": "36b5a79c-d51f-45ad-8fab-24fcc9948789", "value": "purpose"},
			map[string]any{"formFieldId": "d10e07d0-0f87-465e-8d5f-c81b9fff9beb", "value": "2026-01-01"},
			map[string]any{"formFieldId": "6eaffa72-f1cd-4a9f-bb17-4dec8b5449f9", "generalMasterItemId": "00000000-0000-4000-8000-000000000041"},
			map[string]any{"formFieldId": "12547c53-e398-4b25-b9e2-d90230ba1abe", "files": []string{}},
			map[string]any{"formFieldId": "95427a69-4c2c-472e-8799-7653cf1f36cc", "value": ""},
		},
	})
	if response.Status != http.StatusOK {
		t.Fatalf("unknown request property status=%d body=%s", response.Status, response.Body)
	}
}
