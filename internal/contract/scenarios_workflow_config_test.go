package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestContractWorkflowDetailRowAdditionSetting(t *testing.T) {
	env := testutil.New(t, testutil.WithPublicBaseURL("http://punchflow.test"))
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	var slipSectionID string
	var conditionField model.FormField
	for i := range def.SectionList {
		if def.SectionList[i].SectionType == model.SectionTypeSlip {
			slipSectionID = def.SectionList[i].ID
			def.SectionList[i].AllowApproverToAddItems = true
		}
		if def.SectionList[i].SectionType == model.SectionTypeForm && len(def.SectionList[i].FormFields) > 0 && conditionField.ID == "" {
			conditionField = def.SectionList[i].FormFields[0]
		}
	}
	if slipSectionID == "" {
		t.Fatal("expense-claim fixture has no detail section")
	}
	if conditionField.ID == "" {
		t.Fatal("expense-claim fixture has no form field for section conditions")
	}
	var formTarget *model.FormField
	var masterDefaultField *model.FormField
	var formCondition *model.FormField
	var slipSection *model.WorkflowSection
	for i := range def.SectionList {
		section := &def.SectionList[i]
		if section.SectionType == model.SectionTypeSlip && section.Code == "lines" {
			slipSection = section
		}
		for j := range section.FormFields {
			switch section.FormFields[j].Code {
			case "purpose":
				formTarget = &section.FormFields[j]
			case "cost-center":
				masterDefaultField = &section.FormFields[j]
			case "amount":
				formCondition = &section.FormFields[j]
			}
		}
	}
	if formTarget == nil || masterDefaultField == nil || formCondition == nil || slipSection == nil {
		t.Fatal("expense-claim fixture is missing fields for custom validation contract")
	}
	masterDefaultField.DefaultGeneralMasterItemCode = "cc-sales"
	gt100 := "100"
	formTarget.CustomValidations = []model.FormCustomValidation{{
		ID: "00000000-0000-4000-8000-0000000000b5", CombinationType: "all", ErrorMessage: "Amount is too high",
		Fields: []model.FormCustomValidationField{{ID: "00000000-0000-4000-8000-0000000000b6", FormFieldCode: "amount", Symbol: "greater_than", Value: &gt100}},
	}}
	var slipTarget *model.SlipField
	for i := range slipSection.SlipFields {
		if slipSection.SlipFields[i].Code == "item" {
			slipTarget = &slipSection.SlipFields[i]
		}
	}
	if slipTarget == nil {
		t.Fatal("expense-claim detail section has no item field")
	}
	slipTarget.GeneralMasterCode = "cost-centers"
	slipTarget.DefaultGeneralMasterItemCode = "cc-sales"
	slipTarget.CustomValidations = []model.SlipCustomValidation{{
		ID: "00000000-0000-4000-8000-0000000000b7", CombinationType: "any", ErrorMessage: "Quantity is high",
		Fields: []model.SlipCustomValidationField{{ID: "00000000-0000-4000-8000-0000000000b8", SlipFieldCode: "qty", Symbol: "greater_than", Value: &gt100}},
	}}
	def.SectionList = append(def.SectionList, model.WorkflowSection{
		ID: "00000000-0000-4000-8000-0000000000b1", Title: "Custom condition", SectionType: model.SectionTypeForm,
		Conditional: true, CombinationType: "custom", CombinationExpression: "(A || B) && C",
		ConditionFields: []model.ConditionField{
			{ID: "00000000-0000-4000-8000-0000000000b2", Variable: conditionField.Code, Symbol: "equal", Value: "a"},
			{ID: "00000000-0000-4000-8000-0000000000b3", Variable: conditionField.Code, Symbol: "equal", Value: "b"},
			{ID: "00000000-0000-4000-8000-0000000000b4", Variable: conditionField.Code, Symbol: "equal", Value: "c"},
		},
	})
	raw, err := json.Marshal(def)
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
	validateJSON(t, pf16ResponseSchema(t, schemaCompiler(t, loadSpec(t)), operation), response.Body)
	var workflow struct {
		SectionList []struct {
			ID                      string `json:"id"`
			SectionType             string `json:"sectionType"`
			AllowApproverToAddItems bool   `json:"allowApproverToAddItems"`
			CombinationType         string `json:"combinationType"`
			CombinationExpression   string `json:"combinationExpression"`
			SlipFields              []struct {
				Code                     string         `json:"code"`
				GeneralMaster            map[string]any `json:"generalMaster"`
				DefaultGeneralMasterItem map[string]any `json:"defaultGeneralMasterItem"`
			} `json:"slipFields"`
			FormFields []struct {
				Code                     string         `json:"code"`
				GeneralMaster            map[string]any `json:"generalMaster"`
				DefaultGeneralMasterItem map[string]any `json:"defaultGeneralMasterItem"`
			} `json:"formFields"`
		} `json:"sectionList"`
	}
	response.JSON(t, &workflow)
	found, foundMaster, foundDefault, foundSlipDefault := false, false, false, false
	for _, section := range workflow.SectionList {
		for _, field := range section.FormFields {
			if field.Code == "cost-center" && field.GeneralMaster["code"] == "cost-centers" && field.DefaultGeneralMasterItem["code"] == "cc-sales" {
				foundDefault = true
			}
		}
		if section.ID == slipSectionID && section.SectionType == model.SectionTypeSlip {
			if !section.AllowApproverToAddItems {
				t.Fatal("populated detail section did not present allowApproverToAddItems=true")
			}
			for _, field := range section.SlipFields {
				if field.Code == "item" && field.GeneralMaster["code"] == "cost-centers" {
					foundMaster = true
					foundSlipDefault = field.DefaultGeneralMasterItem["code"] == "cc-sales"
				}
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("response omitted the configured detail section")
	}
	if !foundMaster {
		t.Fatal("workflow detail field omitted its populated general-master metadata")
	}
	if !foundDefault {
		t.Fatal("workflow form field omitted its populated default general-master item")
	}
	if !foundSlipDefault {
		t.Fatal("workflow detail field omitted its populated default general-master item")
	}
	var customExpressionFound bool
	for _, section := range workflow.SectionList {
		if section.ID == "00000000-0000-4000-8000-0000000000b1" {
			customExpressionFound = section.CombinationType == "custom" && section.CombinationExpression == "(A || B) && C"
		}
	}
	if !customExpressionFound {
		t.Fatal("workflow response omitted the populated custom section expression")
	}

	// Ticket webhook data uses the same workflow presenter and retains the
	// populated section setting.
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	ticketID := ""
	actorID := env.UserID(t, "sales-rep")
	inputs := make([]engine.FieldInput, 0)
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			inputs = append(inputs, engine.FieldInput{FormFieldID: field.ID, FormFieldCode: field.Code, Value: json.RawMessage("null")})
		}
	}
	if err := env.DB.WithTx(t.Context(), func(tx *store.Tx) error {
		var createErr error
		ticketID, _, createErr = engine.New(env.Clock).Create(t.Context(), tx, engine.Actor{UserID: actorID}, engine.CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflowID, AuthorTeamID: teamID, Inputs: &inputs})
		return createErr
	}); err != nil {
		t.Fatalf("create ticket for workflow webhook: %v", err)
	}
	payloadSchema, err := schemaCompiler(t, loadSpec(t)).Compile("file:///kickflow-openapi.json#/webhooks/ticket/post/requestBody/content/application~1json/schema")
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterReceiverWebhook(t, "config-secret", "ticket_updated")
	fired := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", map[string]any{"eventType": "ticket_updated", "ticketId": ticketID})
	if fired.Status != http.StatusOK {
		t.Fatalf("fire workflow webhook: %d %s", fired.Status, fired.Body)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("deliver workflow webhook attempts=%d, want 1", attempts)
	}
	requests := env.Receiver.Requests()
	if len(requests) == 0 {
		t.Fatal("webhook receiver got no request")
	}
	body := requests[len(requests)-1].Body
	validateJSON(t, payloadSchema, body)
	var payload struct {
		Data struct {
			Ticket struct {
				Workflow struct {
					SectionList []struct {
						ID                      string `json:"id"`
						AllowApproverToAddItems bool   `json:"allowApproverToAddItems"`
					} `json:"sectionList"`
				} `json:"workflow"`
				TicketSections []struct {
					ID      string `json:"id"`
					Visible bool   `json:"visible"`
				} `json:"ticketSections"`
			} `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	for _, section := range payload.Data.Ticket.Workflow.SectionList {
		if section.ID == slipSectionID && section.AllowApproverToAddItems {
			for _, ticketSection := range payload.Data.Ticket.TicketSections {
				if ticketSection.ID == "00000000-0000-4000-8000-0000000000b1" {
					if ticketSection.Visible {
						t.Fatal("ticket presenter marked an unmatched custom conditional section visible")
					}
					return
				}
			}
			t.Fatal("webhook omitted the ticket section evaluated by the custom expression")
		}
	}
	t.Fatal("webhook omitted configured allowApproverToAddItems=true")
}

func TestContractWorkflowRouteConditionResourceThresholds(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	var otherwise model.RouteCondition
	for _, condition := range def.RouteConditions {
		if condition.ConditionType == model.RouteConditionTypeFieldOtherwise {
			otherwise = condition
			break
		}
	}
	branch := model.RouteCondition{
		ID: "00000000-0000-4000-8000-0000000000d1", ConditionType: model.RouteConditionTypeField,
		CombinationType: model.RouteCombinationAll, RouteCode: &def.RouteCode,
		ConditionFields: []model.ConditionField{
			{ID: "00000000-0000-4000-8000-0000000000d2", Variable: "amount", FieldKey: model.StepConditionFieldKeyAuthorTeam, Symbol: "equal", Value: "sales"},
			{ID: "00000000-0000-4000-8000-0000000000d3", Variable: "amount", FieldKey: model.StepConditionFieldKeyAuthorGrade, Symbol: "equal", Value: "staff"},
			{ID: "00000000-0000-4000-8000-0000000000d4", FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "cost-center", Symbol: "equal", GeneralMasterCode: "cost-centers", GeneralMasterItemCode: "cc-eng"},
		},
	}
	def.RouteConditions = append([]model.RouteCondition{branch}, otherwise)
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID); err != nil {
		t.Fatal(err)
	}
	var operation apiv1.Operation
	for _, candidate := range apiv1.Operations() {
		if candidate.OperationID == "getWorkflow" {
			operation = candidate
			break
		}
	}
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get workflow: status=%d body=%s", response.Status, response.Body)
	}
	validateJSON(t, pf16ResponseSchema(t, schemaCompiler(t, loadSpec(t)), operation), response.Body)
	var body struct {
		RouteConditions []struct {
			Fields []map[string]any `json:"conditionFields"`
		} `json:"routeConditions"`
	}
	response.JSON(t, &body)
	fields := body.RouteConditions[0].Fields
	if len(fields) != 3 || fields[0]["team"] == nil || fields[1]["grade"] == nil || fields[2]["generalMasterItem"] == nil || fields[2]["value"] != nil {
		t.Fatalf("route condition resource presentation=%v", fields)
	}
}
