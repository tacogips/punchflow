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

func TestContractWorkflowFormPresentsMasterFiltersAndTicketCopySetting(t *testing.T) {
	env := testutil.New(t, testutil.WithPublicBaseURL("http://punchflow.test"))
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	masterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	masterFields, err := store.GetGeneralMasterFields(t.Context(), env.DB.Q(), masterID)
	if err != nil || len(masterFields) == 0 {
		t.Fatalf("general master fields: %v (count %d)", err, len(masterFields))
	}
	var sourceID, pickerID, slipSourceID, slipPickerID string
	for i := range definition.SectionList {
		section := &definition.SectionList[i]
		for j := range section.FormFields {
			field := &section.FormFields[j]
			switch field.Code {
			case "purpose":
				sourceID = field.ID
			case "cost-center":
				pickerID = field.ID
			}
		}
		for j := range section.SlipFields {
			field := &section.SlipFields[j]
			switch field.Code {
			case "qty":
				slipSourceID = field.ID
			case "item":
				slipPickerID = field.ID
			}
		}
	}
	if sourceID == "" || pickerID == "" || slipSourceID == "" || slipPickerID == "" {
		t.Fatal("expense-claim fixture is missing metadata source/target fields")
	}
	for i := range definition.SectionList {
		section := &definition.SectionList[i]
		for j := range section.FormFields {
			field := &section.FormFields[j]
			if field.Code == "cost-center" {
				field.GeneralMasterSearchFilters = []model.GeneralMasterSearchFilter{{ID: "00000000-0000-4000-8000-0000000000e1", FilterFormFieldID: sourceID, FieldType: "custom_field", GeneralMasterFieldID: &masterFields[0].ID}}
			}
			if field.Code == "receipt" {
				titleKind, target := "ticket_title", "purpose"
				field.FieldType = "ticket"
				field.TicketCopySetting = &model.TicketCopySetting{WorkflowCode: "expense-claim", Mappings: []model.TicketCopyMapping{{FieldType: titleKind, TargetFieldCode: &target}}}
			}
		}
		for j := range section.SlipFields {
			field := &section.SlipFields[j]
			if field.Code == "item" {
				field.GeneralMasterSearchFilters = []model.SlipGeneralMasterSearchFilter{{ID: "00000000-0000-4000-8000-0000000000e2", FilterSlipFieldID: slipSourceID, FieldType: "custom_field", GeneralMasterFieldID: &masterFields[0].ID}}
			}
		}
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.DB.WithTx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID)
		return err
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
	if response.Status != http.StatusOK {
		t.Fatalf("get workflow: %d %s", response.Status, response.Body)
	}
	validateJSON(t, pf16ResponseSchema(t, schemaCompiler(t, loadSpec(t)), operation), response.Body)
	var got struct {
		SectionList []struct {
			FormFields []struct {
				Code                       string           `json:"code"`
				GeneralMasterSearchFilters []map[string]any `json:"generalMasterSearchFilters"`
				TicketCopySetting          map[string]any   `json:"ticketCopySetting"`
			} `json:"formFields"`
			SlipFields []struct {
				Code                       string           `json:"code"`
				GeneralMasterSearchFilters []map[string]any `json:"generalMasterSearchFilters"`
			} `json:"slipFields"`
		} `json:"sectionList"`
	}
	response.JSON(t, &got)
	var formFilter, slipFilter []map[string]any
	var copy map[string]any
	for _, section := range got.SectionList {
		for _, field := range section.FormFields {
			if field.Code == "cost-center" {
				formFilter = field.GeneralMasterSearchFilters
			}
			if field.Code == "receipt" {
				copy = field.TicketCopySetting
			}
		}
		for _, field := range section.SlipFields {
			if field.Code == "item" {
				slipFilter = field.GeneralMasterSearchFilters
			}
		}
	}
	if len(formFilter) != 1 || formFilter[0]["filterFormFieldId"] != sourceID {
		t.Fatalf("form filters=%v", formFilter)
	}
	if len(slipFilter) != 1 || slipFilter[0]["filterSlipFieldId"] != slipSourceID {
		t.Fatalf("slip filters=%v", slipFilter)
	}
	if copy == nil || len(copy["mappings"].([]any)) != 1 || copy["workflow"].(map[string]any)["id"] != workflowID {
		t.Fatalf("ticket copy setting=%v", copy)
	}
}
