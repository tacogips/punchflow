package contract_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
	"punchflow/internal/webhook"
)

func TestContractWebhookSchemas(t *testing.T) {
	spec := loadSpec(t)
	c := schemaCompiler(t, spec)
	webhooks, ok := spec["webhooks"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI webhooks is not an object")
	}
	if len(model.WebhookEventTypes) != 13 {
		t.Fatalf("event type count = %d, want 13", len(model.WebhookEventTypes))
	}
	for _, event := range model.WebhookEventTypes {
		event := event
		t.Run(event, func(t *testing.T) {
			name := "ticket"
			if event == "ping" {
				name = "ping"
			} else if strings.HasPrefix(event, "comment_") {
				name = "comment"
			}
			hook, ok := webhooks[name].(map[string]any)
			if !ok {
				t.Fatalf("missing webhook schema %q", name)
			}
			if _, err := c.Compile("file:///kickflow-openapi.json#/webhooks/" + name + "/post/requestBody/content/application~1json/schema"); err != nil {
				t.Fatalf("compile %s payload schema: %v", event, err)
			}
			_ = hook
		})
	}
}

func TestContractWebhookPayloads(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	c := schemaCompiler(t, spec)
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]engine.FieldInput, 0)
	for _, section := range definition.SectionList {
		for _, field := range section.FormFields {
			inputs = append(inputs, engine.FieldInput{FormFieldID: field.ID, FormFieldCode: field.Code, Value: json.RawMessage("null")})
		}
	}
	actorID := env.UserID(t, "sales-rep")
	ticketID := ""
	err = env.DB.WithTx(t.Context(), func(tx *store.Tx) error {
		var createErr error
		ticketID, _, createErr = engine.New(env.Clock).Create(t.Context(), tx, engine.Actor{UserID: actorID}, engine.CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflowID, AuthorTeamID: teamID, Inputs: &inputs})
		return createErr
	})
	if err != nil {
		t.Fatalf("create webhook fixture ticket: %v", err)
	}
	commentID := store.NewID()
	now := store.Millis(env.Clock.Now())
	if _, err := env.DB.Q().ExecContext(t.Context(), "INSERT INTO comments(id,ticket_id,user_id,body,mentioned_user_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", commentID, ticketID, env.UserID(t, "admin"), "contract comment", "[]", now, now); err != nil {
		t.Fatal(err)
	}
	secret := "pf26-contract-secret"
	webhookID := env.RegisterReceiverWebhook(t, secret)
	seen := make(map[string]bool, len(model.WebhookEventTypes))
	for _, event := range model.WebhookEventTypes {
		event := event
		t.Run(event, func(t *testing.T) {
			body := map[string]any{"eventType": event, "ticketId": ticketID}
			if event == model.WebhookEventPing {
				body = map[string]any{"eventType": event, "webhookIds": []string{webhookID}}
			}
			if strings.HasPrefix(event, "comment_") {
				body["commentId"] = commentID
			}
			fired := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", body)
			if fired.Status != http.StatusOK {
				t.Fatalf("fire %s: %d %s", event, fired.Status, fired.Body)
			}
			before := len(env.Receiver.Requests())
			if attempts := env.DeliverWebhooks(t); attempts != 1 {
				t.Fatalf("deliver %s attempts=%d, want 1", event, attempts)
			}
			requests := env.Receiver.Requests()
			if len(requests) != before+1 {
				t.Fatalf("receiver request count=%d, want %d", len(requests), before+1)
			}
			received := requests[len(requests)-1]
			if !webhook.Verify([]byte(secret), received.Body, received.Header.Get("X-Kickflow-Signature")) {
				t.Fatalf("%s signature invalid", event)
			}
			var payload map[string]any
			if err := json.Unmarshal(received.Body, &payload); err != nil {
				t.Fatalf("decode %s payload: %v", event, err)
			}
			if payload["eventType"] != event {
				t.Fatalf("payload eventType=%v, want %s", payload["eventType"], event)
			}
			seen[event] = true
			name := "ticket"
			if event == model.WebhookEventPing {
				name = "ping"
			} else if strings.HasPrefix(event, "comment_") {
				name = "comment"
			}
			schema, err := c.Compile("file:///kickflow-openapi.json#/webhooks/" + name + "/post/requestBody/content/application~1json/schema")
			if err != nil {
				t.Fatalf("compile %s schema: %v", event, err)
			}
			validateJSON(t, schema, received.Body)
			var decoded any
			if err := json.Unmarshal(received.Body, &decoded); err != nil {
				t.Fatal(err)
			}
			checkTimestamps(t, decoded)
		})
	}
	if len(seen) != len(model.WebhookEventTypes) {
		t.Fatalf("received event set size=%d, want %d", len(seen), len(model.WebhookEventTypes))
	}
	for _, event := range model.WebhookEventTypes {
		if !seen[event] {
			t.Errorf("missing event %s", event)
		}
	}
}
