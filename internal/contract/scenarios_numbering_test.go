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
	created := pf26aCreateTicket(t, env, createOperation, map[string]*jsonschema.Schema{"createTicket": createSchema}, spec, model.TicketStatusDraft)
	ticketID := created.ID
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	keyID := "00000000-0000-4000-8000-0000000000c1"
	customID := "00000000-0000-4000-8000-0000000000c2"
	settingID := "00000000-0000-4000-8000-0000000000c3"
	format := "CN-[number]"
	definition.TicketNumberKey = &model.TicketNumberKey{ID: keyID, Code: "SHARED", Name: "Shared group", Default: true}
	definition.TicketNumberFormat = pf25String("WF-[number]")
	definition.TicketNumberingTiming = "created"
	definition.CustomNumberings = []model.CustomNumbering{{ID: customID, Code: "DECISION", Name: "Decision number", Value: 1}}
	definition.CustomNumberingSettings = []model.CustomNumberingSetting{{ID: settingID, CustomNumberingID: customID, TicketNumberFormat: &format, TimingType: "created", SortOrder: 1}}
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
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET ticket_number='WF-1' WHERE id=?`, ticketID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_number_key_counters(ticket_number_key_id,counter) VALUES(?,1)`, keyID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO custom_numbering_counters(custom_numbering_id,counter) VALUES(?,1)`, "custom:"+customID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_custom_numbering_values(ticket_id,custom_numbering_id,formatted_number) VALUES(?,?,?)`, ticketID, customID, "CN-1"); err != nil {
		t.Fatal(err)
	}
	workflowResponse := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	validateJSON(t, workflowSchema, workflowResponse.Body)
	var workflow struct {
		TicketNumberKey         map[string]any   `json:"ticketNumberKey"`
		CustomNumberings        []map[string]any `json:"customNumberings"`
		CustomNumberingSettings []map[string]any `json:"customNumberingSettings"`
	}
	workflowResponse.JSON(t, &workflow)
	if workflow.TicketNumberKey["id"] != keyID || len(workflow.CustomNumberings) != 1 || len(workflow.CustomNumberingSettings) != 1 {
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

	payloadSchema, err := compiler.Compile("file:///kickflow-openapi.json#/webhooks/ticket/post/requestBody/content/application~1json/schema")
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterReceiverWebhook(t, "numbering-secret", "ticket_updated")
	fired := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", map[string]any{"eventType": "ticket_updated", "ticketId": ticketID})
	if fired.Status != http.StatusOK {
		t.Fatalf("fire ticket webhook: %d %s", fired.Status, fired.Body)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("webhook delivery attempts=%d, want 1", attempts)
	}
	requests := env.Receiver.Requests()
	if len(requests) != 1 {
		t.Fatalf("webhook requests=%d, want 1", len(requests))
	}
	validateJSON(t, payloadSchema, requests[0].Body)
	var webhook struct {
		Data struct {
			Ticket struct {
				CustomNumberingValues []struct {
					FormattedNumber string `json:"formattedNumber"`
				} `json:"customNumberingValues"`
			} `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(requests[0].Body, &webhook); err != nil {
		t.Fatal(err)
	}
	if len(webhook.Data.Ticket.CustomNumberingValues) != 1 || webhook.Data.Ticket.CustomNumberingValues[0].FormattedNumber != "CN-1" {
		t.Fatalf("webhook custom numbering values=%+v", webhook.Data.Ticket.CustomNumberingValues)
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
