package contract_test

import (
	"net/http"
	"net/url"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// N04: GET /v1/tickets/tasks honours subStatusIds[] and reports the filtered
// total, like GET /v1/tickets.
func TestContractTaskListSubStatusFilter(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	createSchema, err := compiler.Compile(responseSchemaURI(operations["createTicket"].Pattern, "post"))
	if err != nil {
		t.Fatal(err)
	}
	tasksSchema, err := compiler.Compile(responseSchemaURI(operations["listTasks"].Pattern, "get"))
	if err != nil {
		t.Fatal(err)
	}
	schemas := map[string]*jsonschema.Schema{"createTicket": createSchema}
	first := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress")
	second := pf26aCreateTicket(t, env, operations["createTicket"], schemas, spec, "in_progress")

	underReview, err := store.SubStatusByCode(t.Context(), env.DB.Q(), "under-review")
	if err != nil {
		t.Fatal(err)
	}
	financeCheck, err := store.SubStatusByCode(t.Context(), env.DB.Q(), "finance-check")
	if err != nil {
		t.Fatal(err)
	}
	// Both tickets wait for the manager under "under-review"; move one to
	// "finance-check" so the manager's task list spans two sub-statuses.
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET sub_status_id=? WHERE id=?`, financeCheck.ID, second.ID); err != nil {
		t.Fatal(err)
	}

	list := func(subStatusID string) ([]string, string) {
		t.Helper()
		path := "/v1/tickets/tasks"
		if subStatusID != "" {
			path += "?" + url.Values{"subStatusIds[]": {subStatusID}}.Encode()
		}
		response := env.Do(t, testutil.Manager, http.MethodGet, path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("list tasks (%q): %d %s", subStatusID, response.Status, response.Body)
		}
		validateJSON(t, tasksSchema, response.Body)
		var tickets []struct {
			ID string `json:"id"`
		}
		response.JSON(t, &tickets)
		ids := make([]string, 0, len(tickets))
		for _, item := range tickets {
			ids = append(ids, item.ID)
		}
		return ids, response.Header.Get("Total")
	}

	all, total := list("")
	if len(all) != 2 || total != "2" {
		t.Fatalf("unfiltered tasks = %v total=%s, want both tickets", all, total)
	}
	ids, total := list(underReview.ID)
	if len(ids) != 1 || ids[0] != first.ID || total != "1" {
		t.Fatalf("under-review tasks = %v total=%s, want only %s", ids, total, first.ID)
	}
	ids, total = list(financeCheck.ID)
	if len(ids) != 1 || ids[0] != second.ID || total != "1" {
		t.Fatalf("finance-check tasks = %v total=%s, want only %s", ids, total, second.ID)
	}
}
