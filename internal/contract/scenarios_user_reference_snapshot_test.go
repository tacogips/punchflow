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

const f5SelectedUserFieldID = "00000000-0000-4000-8000-0000000005f1"

func TestContractTicketSelectedUserSnapshotCustomFields(t *testing.T) {
	env := testutil.New(t, testutil.WithPublicBaseURL("http://punchflow.test"))
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	selectedUser := model.FormField{ID: f5SelectedUserFieldID, Code: "selected-user", Title: "Selected user", FieldType: "user", Size: "full", Orientation: "horizontal", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}
	for i := range def.SectionList {
		if def.SectionList[i].SectionType == model.SectionTypeForm {
			def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, selectedUser)
			break
		}
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
	salesTeamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := store.ResolveID(t.Context(), env.DB.Q(), "users", "admin")
	if err != nil {
		t.Fatal(err)
	}
	costCentersID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	costCenterItemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), costCentersID, "cc-sales")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE teams SET name='Sales before submit' WHERE id=?`, salesTeamID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE general_master_items SET name='Cost center before submit' WHERE id=?`, costCenterItemID); err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": "draft", "workflowId": workflowID, "authorTeamId": salesTeamID, "title": "F5 user snapshot contract",
		"inputs": []map[string]any{
			{"formFieldCode": "amount", "value": 10}, {"formFieldCode": "purpose", "value": "snapshot"},
			{"formFieldCode": "expense-date", "value": "2026-01-01"}, {"formFieldCode": "cost-center", "generalMasterItemId": costCenterItemID},
			{"formFieldCode": "receipt", "files": []string{}}, {"formFieldCode": "approver-note", "value": ""},
			{"formFieldCode": "selected-user", "userId": adminID},
		},
	})
	var createdTicket struct {
		ID string `json:"id"`
	}
	created.JSON(t, &createdTicket)
	if createdTicket.ID == "" {
		t.Fatalf("create ticket returned no id: %s", created.Body)
	}
	status := "in_progress"
	env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+createdTicket.ID, map[string]any{"status": status})
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	spec := loadSpec(t)
	get := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+createdTicket.ID, nil)
	pf26aCheck(t, operations["getTicket"], get, pf16ResponseSchema(t, schemaCompiler(t, spec), operations["getTicket"]), spec)
	var ticket struct {
		AuthorTeamSnapshot map[string]any `json:"authorTeamSnapshot"`
		Inputs             []struct {
			FormField map[string]any `json:"formField"`
			Value     any            `json:"value"`
		} `json:"inputs"`
	}
	get.JSON(t, &ticket)
	if ticket.AuthorTeamSnapshot["name"] != "Sales before submit" {
		t.Errorf("submission applicant-team snapshot=%v", ticket.AuthorTeamSnapshot)
	}
	foundMaster := false
	foundUser := false
	for _, input := range ticket.Inputs {
		if input.FormField["code"] == "cost-center" {
			items, ok := input.Value.([]any)
			if !ok || len(items) != 1 {
				t.Errorf("submission master snapshot=%v", input.Value)
			} else if item, ok := items[0].(map[string]any); !ok || item["name"] != "Cost center before submit" {
				t.Errorf("submission master snapshot=%v", input.Value)
			}
			foundMaster = true
		}
		if input.FormField["code"] != "selected-user" {
			continue
		}
		value, ok := input.Value.([]any)
		if !ok || len(value) != 1 {
			t.Fatalf("selected-user cached value=%v", input.Value)
		}
		user, ok := value[0].(map[string]any)
		if !ok {
			t.Fatalf("selected-user snapshot=%#v", value[0])
		}
		customFields, ok := user["customFields"].(map[string]any)
		if !ok || customFields["cost-center"] != "CC-100" {
			t.Fatalf("selected-user public customFields=%v", user["customFields"])
		}
		if _, leaked := customFields["hire-date"]; leaked {
			t.Fatalf("private custom field leaked in public ticket snapshot: %v", customFields)
		}
		foundUser = true
	}
	if !foundMaster {
		t.Errorf("getTicket omitted master reference: %s", get.Body)
	}
	if !foundUser {
		t.Errorf("getTicket omitted selected-user field: %s", get.Body)
	}
}
