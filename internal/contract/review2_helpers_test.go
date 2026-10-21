package contract_test

import (
	"encoding/json"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// n03Body builds a valid draft createTicket request for the expense-claim workflow.
func n03Body(t *testing.T, env *testutil.Env) map[string]any {
	t.Helper()
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatalf("resolve expense-claim workflow: %v", err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatalf("resolve sales team: %v", err)
	}
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatalf("resolve cost-centers master: %v", err)
	}
	itemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatalf("resolve cc-sales master item: %v", err)
	}
	fields := pf26aFormFields(t, env)
	return map[string]any{
		"status": "draft", "workflowId": workflowID, "authorTeamId": teamID, "title": "Review round 2",
		"inputs": []map[string]any{
			{"formFieldId": fields["amount"], "value": "5000"},
			{"formFieldId": fields["purpose"], "value": "Review round 2"},
			{"formFieldId": fields["expense-date"], "value": "2026-01-01"},
			{"formFieldId": fields["cost-center"], "generalMasterItemId": itemID},
			{"formFieldId": fields["receipt"], "files": []string{}},
			{"formFieldId": fields["approver-note"], "value": ""},
		},
	}
}

// validateAny validates a Go value (after a JSON round trip) against schema.
func validateAny(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	validateJSON(t, schema, data)
}
