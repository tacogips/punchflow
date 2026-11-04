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

func TestContractWorkflowCustomValidationResourceThresholds(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "purchase-request")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	foundTarget := false
	for si := range def.SectionList {
		for fi := range def.SectionList[si].FormFields {
			field := &def.SectionList[si].FormFields[fi]
			if field.Code != "item" {
				continue
			}
			field.CustomValidations = []model.FormCustomValidation{{
				ID: "00000000-0000-4000-8000-0000000000c1", CombinationType: "all", ErrorMessage: "resource condition matched",
				Fields: []model.FormCustomValidationField{
					{ID: "00000000-0000-4000-8000-0000000000c2", FormFieldCode: "approving-team", Symbol: "descendants_or_equal", TeamCode: "sales"},
					{ID: "00000000-0000-4000-8000-0000000000c3", FormFieldCode: "reviewer", Symbol: "equal", GradeCode: "manager"},
					{ID: "00000000-0000-4000-8000-0000000000c4", FormFieldCode: "vendor", Symbol: "equal", GeneralMasterCode: "vendors", GeneralMasterItemCode: "vendor-acme"},
				},
			}}
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Fatal("purchase-request fixture has no item field")
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID); err != nil {
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
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/purchase-request", nil)
	validateJSON(t, pf16ResponseSchema(t, schemaCompiler(t, loadSpec(t)), operation), response.Body)
	var body struct {
		SectionList []struct {
			FormFields []struct {
				Code              string `json:"code"`
				CustomValidations []struct {
					Fields []map[string]any `json:"fields"`
				} `json:"customValidations"`
			} `json:"formFields"`
		} `json:"sectionList"`
	}
	response.JSON(t, &body)
	for _, section := range body.SectionList {
		for _, field := range section.FormFields {
			if field.Code != "item" || len(field.CustomValidations) == 0 {
				continue
			}
			conditions := field.CustomValidations[0].Fields
			if len(conditions) != 3 || conditions[0]["team"].(map[string]any)["code"] != "sales" || conditions[1]["grade"].(map[string]any)["code"] != "manager" || conditions[2]["generalMasterItem"].(map[string]any)["code"] != "vendor-acme" {
				t.Fatalf("custom validation resource thresholds=%v", conditions)
			}
			return
		}
	}
	t.Fatal("getWorkflow omitted the populated custom validations")
}
