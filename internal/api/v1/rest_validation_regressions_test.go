package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestMissingRequiredPropertiesReturnBadRequest(t *testing.T) {
	env := testutil.New(t)
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"status", map[string]any{"workflowId": pf22ExpenseWorkflow, "authorTeamId": teamID}},
		{"workflowId", map[string]any{"status": "draft", "authorTeamId": teamID}},
		{"authorTeamId", map[string]any{"status": "draft", "workflowId": pf22ExpenseWorkflow}},
		{"null workflowId", map[string]any{"status": "draft", "workflowId": nil, "authorTeamId": teamID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", tc.body), http.StatusBadRequest, "invalid_parameter")
		})
	}

	ticketID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	comment := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/comments", map[string]any{"body": "before"})
	if comment.Status != http.StatusOK {
		t.Fatalf("create comment=%d %s", comment.Status, comment.Body)
	}
	var commentBody map[string]any
	comment.JSON(t, &commentBody)
	for _, body := range []map[string]any{{}, {"body": nil}} {
		testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticketID+"/comments/"+commentBody["id"].(string), body), http.StatusBadRequest, "invalid_parameter")
	}

	master := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{
		"name":   "Required field check",
		"fields": []map[string]any{{"code": "field", "required": true, "fieldType": "text"}},
	})
	testutil.AssertError(t, master, http.StatusBadRequest, "invalid_parameter")
}

func TestTicketFiltersRejectInvalidSubStatusUUIDs(t *testing.T) {
	env := testutil.New(t)
	for _, path := range []string{
		"/v1/tickets?subStatusIds[]=not-a-uuid",
		"/v1/tickets/tasks?subStatusIds[]=not-a-uuid",
	} {
		testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, path, nil), http.StatusBadRequest, "invalid_parameter")
	}
}
