package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestContractWorkflowApproverRequiredRouteSteps(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	routeID, err := store.RouteIDByCode(t.Context(), env.DB.Q(), def.RouteCode)
	if err != nil {
		t.Fatal(err)
	}
	route, err := store.RouteDefByID(t.Context(), env.DB.Q(), routeID)
	if err != nil || len(route.Steps) == 0 {
		t.Fatalf("load route steps: route=%+v err=%v", route, err)
	}
	stepCode := route.Steps[0].Code
	configured := false
	for i := range def.SectionList {
		for j := range def.SectionList[i].FormFields {
			field := &def.SectionList[i].FormFields[j]
			if field.Code == "approver-note" {
				field.Approver = true
				field.ApproverEditRestriction = &model.ApproverEditRestriction{RouteStepCodes: []string{stepCode}}
				configured = true
			}
		}
	}
	if !configured {
		t.Fatal("expense-claim fixture has no approver-note field")
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.DB.WithTx(t.Context(), func(tx *store.Tx) error {
		_, e := tx.ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var operation apiv1.Operation
	for _, candidate := range apiv1.Operations() {
		if candidate.OperationID == "getWorkflow" {
			operation = candidate
			break
		}
	}
	if operation.OperationID == "" {
		t.Fatal("getWorkflow operation is not registered")
	}
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	validateJSON(t, pf16ResponseSchema(t, schemaCompiler(t, loadSpec(t)), operation), response.Body)
	var workflow struct {
		SectionList []struct {
			FormFields []struct {
				Code                    string `json:"code"`
				ApproverEditRestriction *struct {
					RouteStepCodes []string `json:"routeStepCodes"`
				} `json:"approverEditRestriction"`
			} `json:"formFields"`
		} `json:"sectionList"`
	}
	response.JSON(t, &workflow)
	for _, section := range workflow.SectionList {
		for _, field := range section.FormFields {
			if field.Code == "approver-note" {
				if field.ApproverEditRestriction == nil || len(field.ApproverEditRestriction.RouteStepCodes) != 1 || field.ApproverEditRestriction.RouteStepCodes[0] != stepCode {
					t.Fatalf("approverEditRestriction=%+v, want configured route step %q", field.ApproverEditRestriction, stepCode)
				}
				return
			}
		}
	}
	t.Fatal("workflow response omitted approver-note field")
}
