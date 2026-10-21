package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestContractTicketNumberingGroupAndCustomValues(t *testing.T) {
	env := testutil.New(t, testutil.WithPublicBaseURL("http://punchflow.test"))
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	workflowOperation := pf25Operation(t, "getWorkflow")
	ticketOperation := pf25Operation(t, "getTicket")
	createOperation := pf25Operation(t, "createTicket")
	workflowSchema := pf16ResponseSchema(t, compiler, workflowOperation)
	ticketSchema := pf16ResponseSchema(t, compiler, ticketOperation)
	createSchema := pf16ResponseSchema(t, compiler, createOperation)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	keyID := "00000000-0000-4000-8000-0000000000c1"
	customID := "00000000-0000-4000-8000-0000000000c2"
	settingID := "00000000-0000-4000-8000-0000000000c3"
	openedCustomID := "00000000-0000-4000-8000-0000000000c4"
	openedSettingID := "00000000-0000-4000-8000-0000000000c5"
	completedCustomID := "00000000-0000-4000-8000-0000000000c6"
	completedSettingID := "00000000-0000-4000-8000-0000000000c7"
	subStatusCustomID := "00000000-0000-4000-8000-0000000000c8"
	subStatusSettingID := "00000000-0000-4000-8000-0000000000c9"
	format := "CN-[number]"
	openedFormat := "ON-[number]"
	completedFormat := "DN-[number]"
	subStatusFormat := "SN-[number]"
	underReview, err := store.SubStatusByCode(t.Context(), env.DB.Q(), "under-review")
	if err != nil {
		t.Fatal(err)
	}
	subStatusID := underReview.ID
	definition.TicketNumberKey = &model.TicketNumberKey{ID: keyID, Code: "SHARED", Name: "Shared group", Default: true}
	definition.TicketNumberFormat = pf25String("WF-[number]")
	definition.TicketNumberingTiming = "created"
	definition.CustomNumberings = []model.CustomNumbering{{ID: customID, Code: "DECISION", Name: "Decision number", Value: 1}, {ID: openedCustomID, Code: "OPENED", Name: "Opened number", Value: 1}, {ID: completedCustomID, Code: "COMPLETED", Name: "Completed number", Value: 1}, {ID: subStatusCustomID, Code: "SUBSTATUS", Name: "Sub-status number", Value: 1}}
	definition.CustomNumberingSettings = []model.CustomNumberingSetting{{ID: settingID, CustomNumberingID: customID, TicketNumberFormat: &format, TimingType: "created", SortOrder: 1}, {ID: openedSettingID, CustomNumberingID: openedCustomID, TicketNumberFormat: &openedFormat, TimingType: "opened", SortOrder: 2}, {ID: completedSettingID, CustomNumberingID: completedCustomID, TicketNumberFormat: &completedFormat, TimingType: "completed", SortOrder: 3}, {ID: subStatusSettingID, CustomNumberingID: subStatusCustomID, TicketNumberFormat: &subStatusFormat, TimingType: "sub_status_attached", SubStatusID: &subStatusID, SortOrder: 4}}
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
	env.RegisterReceiverWebhook(t, "numbering-secret", "ticket_created", "ticket_opened", "ticket_completed")
	createSchemas := map[string]*jsonschema.Schema{"createTicket": createSchema}
	draft := pf26aCreateTicket(t, env, createOperation, createSchemas, spec, model.TicketStatusDraft)
	ticketID := draft.ID
	workflowResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	validateJSON(t, workflowSchema, workflowResponse.Body)
	var workflow struct {
		TicketNumberKey         map[string]any   `json:"ticketNumberKey"`
		CustomNumberings        []map[string]any `json:"customNumberings"`
		CustomNumberingSettings []map[string]any `json:"customNumberingSettings"`
	}
	workflowResponse.JSON(t, &workflow)
	if workflow.TicketNumberKey["id"] != keyID || len(workflow.CustomNumberings) != 4 || len(workflow.CustomNumberingSettings) != 4 {
		t.Fatalf("workflow numbering config=%+v", workflow)
	}

	ticketResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticketID, nil)
	validateJSON(t, ticketSchema, ticketResponse.Body)
	var ticket struct {
		TicketNumber          string `json:"ticketNumber"`
		CustomNumberingValues []struct {
			FormattedNumber string `json:"formattedNumber"`
			CustomNumbering struct {
				Code string `json:"code"`
			} `json:"customNumbering"`
		} `json:"customNumberingValues"`
	}
	ticketResponse.JSON(t, &ticket)
	if ticket.TicketNumber != "WF-1" || len(ticket.CustomNumberingValues) != 1 || ticket.CustomNumberingValues[0].FormattedNumber != "CN-1" || ticket.CustomNumberingValues[0].CustomNumbering.Code != "DECISION" {
		t.Fatalf("ticket numbering=%+v", ticket)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("draft creation webhook delivery attempts=%d, want 1", attempts)
	}
	assertNumberingWebhook(t, env.Receiver.Requests()[0].Body, payloadSchemaForTest(t, compiler), map[string]string{"DECISION": "CN-1"})

	// Direct submission must allocate creation-timed and opened-timed values
	// through the same POST path, and expose both values in the REST and webhook
	// payloads.
	submitted := pf26aCreateTicket(t, env, createOperation, createSchemas, spec, model.TicketStatusInProgress)
	submittedResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+submitted.ID, nil)
	if submittedResponse.Status != http.StatusOK {
		t.Fatalf("get directly submitted ticket: %d %s", submittedResponse.Status, submittedResponse.Body)
	}
	var submittedTicket struct {
		TicketNumber          string `json:"ticketNumber"`
		CustomNumberingValues []struct {
			FormattedNumber string `json:"formattedNumber"`
			CustomNumbering struct {
				Code string `json:"code"`
			} `json:"customNumbering"`
		} `json:"customNumberingValues"`
	}
	submittedResponse.JSON(t, &submittedTicket)
	if submittedTicket.TicketNumber != "WF-2" || len(submittedTicket.CustomNumberingValues) != 3 {
		t.Fatalf("direct submission numbering=%+v", submittedTicket)
	}
	submittedNumbers := make(map[string]string, len(submittedTicket.CustomNumberingValues))
	for _, value := range submittedTicket.CustomNumberingValues {
		submittedNumbers[value.CustomNumbering.Code] = value.FormattedNumber
	}
	if submittedNumbers["DECISION"] != "CN-2" || submittedNumbers["OPENED"] != "ON-1" || submittedNumbers["SUBSTATUS"] != "SN-1" {
		t.Fatalf("direct submission custom numbering=%v, want DECISION=CN-2, OPENED=ON-1, and SUBSTATUS=SN-1", submittedNumbers)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 2 {
		t.Fatalf("direct submission webhook delivery attempts=%d, want 2", attempts)
	}
	requests := env.Receiver.Requests()
	if len(requests) != 3 {
		t.Fatalf("numbering webhook requests=%d, want 3", len(requests))
	}
	assertNumberingWebhook(t, requests[1].Body, payloadSchemaForTest(t, compiler), map[string]string{"DECISION": "CN-2", "OPENED": "ON-1", "SUBSTATUS": "SN-1"})
	assertNumberingWebhook(t, requests[2].Body, payloadSchemaForTest(t, compiler), map[string]string{"DECISION": "CN-2", "OPENED": "ON-1", "SUBSTATUS": "SN-1"})

	for _, actor := range []testutil.Persona{testutil.Manager, testutil.Finance} {
		approved := env.Do(t, actor, http.MethodPost, "/v1/tickets/"+submitted.ID+"/approve", nil)
		if approved.Status != http.StatusOK {
			t.Fatalf("approve direct submission as %q: %d %s", actor, approved.Status, approved.Body)
		}
	}
	completedResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+submitted.ID, nil)
	if completedResponse.Status != http.StatusOK {
		t.Fatalf("get completed ticket: %d %s", completedResponse.Status, completedResponse.Body)
	}
	var completedTicket struct {
		Status                string `json:"status"`
		CustomNumberingValues []struct {
			FormattedNumber string `json:"formattedNumber"`
			CustomNumbering struct {
				Code string `json:"code"`
			} `json:"customNumbering"`
		} `json:"customNumberingValues"`
	}
	completedResponse.JSON(t, &completedTicket)
	if completedTicket.Status != model.TicketStatusCompleted || len(completedTicket.CustomNumberingValues) != 4 {
		t.Fatalf("completed ticket numbering=%+v", completedTicket)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("completion webhook delivery attempts=%d, want 1", attempts)
	}
	requests = env.Receiver.Requests()
	if len(requests) != 4 {
		t.Fatalf("numbering webhook requests=%d after completion, want 4", len(requests))
	}
	assertNumberingWebhook(t, requests[3].Body, payloadSchemaForTest(t, compiler), map[string]string{"DECISION": "CN-2", "OPENED": "ON-1", "SUBSTATUS": "SN-1", "COMPLETED": "DN-1"})
}

func payloadSchemaForTest(t *testing.T, compiler *jsonschema.Compiler) *jsonschema.Schema {
	t.Helper()
	schema, err := compiler.Compile("file:///kickflow-openapi.json#/webhooks/ticket/post/requestBody/content/application~1json/schema")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func assertNumberingWebhook(t *testing.T, body []byte, schema *jsonschema.Schema, want map[string]string) {
	t.Helper()
	validateJSON(t, schema, body)
	var webhook struct {
		Data struct {
			Ticket struct {
				CustomNumberingValues []struct {
					FormattedNumber string `json:"formattedNumber"`
					CustomNumbering struct {
						Code string `json:"code"`
					} `json:"customNumbering"`
				} `json:"customNumberingValues"`
			} `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &webhook); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(webhook.Data.Ticket.CustomNumberingValues))
	for _, value := range webhook.Data.Ticket.CustomNumberingValues {
		got[value.CustomNumbering.Code] = value.FormattedNumber
	}
	if len(got) != len(want) {
		t.Fatalf("webhook custom numbering values=%v, want %v", got, want)
	}
	for code, number := range want {
		if got[code] != number {
			t.Errorf("webhook custom numbering %s=%q, want %q", code, got[code], number)
		}
	}
}

func pf25Operation(t *testing.T, id string) apiv1.Operation {
	t.Helper()
	for _, operation := range apiv1.Operations() {
		if operation.OperationID == id {
			return operation
		}
	}
	t.Fatalf("operation %q is not registered", id)
	return apiv1.Operation{}
}

func pf25String(value string) *string { return &value }
