package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
	"punchflow/internal/webhook"
)

func TestWebhooksCreatePingAndRedaction(t *testing.T) {
	env := testutil.New(t)
	var delivered atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { delivered.Add(1); w.WriteHeader(http.StatusOK) }))
	defer receiver.Close()
	secret := "test-secret"
	created := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhooks", map[string]any{"name": "local", "url": receiver.URL, "secret": secret, "enabled": true, "eventTypes": []string{}})
	if created.Status != http.StatusOK {
		t.Fatalf("create webhook: %d %s", created.Status, created.Body)
	}
	var response map[string]any
	created.JSON(t, &response)
	id, _ := response["id"].(string)
	if id == "" || response["secret"] != secret {
		t.Fatalf("create response missing secret/id: %s", created.Body)
	}
	if n := env.DeliverWebhooks(t); n != 1 || delivered.Load() != 1 {
		t.Fatalf("create ping attempts=%d receiver=%d", n, delivered.Load())
	}
	listed := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhooks", nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("list: %d %s", listed.Status, listed.Body)
	}
	var rows []map[string]any
	listed.JSON(t, &rows)
	if len(rows) != 1 || rows[0]["hasSecret"] != true {
		t.Fatalf("webhook list shape: %s", listed.Body)
	}
	if _, ok := rows[0]["secret"]; ok {
		t.Fatal("list exposed secret")
	}
	bad := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhooks", map[string]any{"name": "bad", "url": receiver.URL, "eventTypes": []string{"ticket_submitted"}})
	testutil.AssertError(t, bad, http.StatusUnprocessableEntity, "validation_failed")
}
func TestWebhookFireEventTypesAndNegativeEvents(t *testing.T) {
	env := testutil.New(t)
	workflow, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	ticketID := ""
	actorID := env.UserID(t, "sales-rep")
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflow)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]engine.FieldInput, 0)
	for _, section := range definition.SectionList {
		for _, field := range section.FormFields {
			inputs = append(inputs, engine.FieldInput{FormFieldID: field.ID, FormFieldCode: field.Code, Value: json.RawMessage("null")})
		}
	}
	err = env.DB.WithTx(t.Context(), func(tx *store.Tx) error {
		var e error
		ticketID, _, e = engine.New(env.Clock).Create(t.Context(), tx, engine.Actor{UserID: actorID}, engine.CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflow, AuthorTeamID: team, Inputs: &inputs})
		return e
	})
	if err != nil {
		t.Fatalf("create test ticket: %v", err)
	}
	commentID := store.NewID()
	if _, err = env.DB.Q().ExecContext(t.Context(), "INSERT INTO comments(id,ticket_id,user_id,body,mentioned_user_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", commentID, ticketID, env.UserID(t, "admin"), "test", "[]", store.Millis(env.Clock.Now()), store.Millis(env.Clock.Now())); err != nil {
		t.Fatal(err)
	}
	secret := "fire-secret"
	webhookID := env.RegisterReceiverWebhook(t, secret)
	if attempts := env.DeliverWebhooks(t); attempts != 0 {
		t.Fatalf("unexpected deliveries before fire cases: %d", attempts)
	}
	for _, event := range model.WebhookEventTypes {
		t.Run(event, func(t *testing.T) {
			before := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhooks/"+webhookID+"/events?perPage=100", nil)
			if before.Status != http.StatusOK {
				t.Fatalf("list events before fire %s: %d %s", event, before.Status, before.Body)
			}
			var beforeRows []map[string]any
			before.JSON(t, &beforeRows)
			beforeIDs := make(map[string]struct{}, len(beforeRows))
			for _, row := range beforeRows {
				if id, ok := row["id"].(string); ok {
					beforeIDs[id] = struct{}{}
				}
			}
			receiverBaseline := len(env.Receiver.Requests())
			body := map[string]any{"eventType": event, "ticketId": ticketID}
			if event == model.WebhookEventPing {
				body["webhookIds"] = []string{webhookID}
				delete(body, "ticketId")
			}
			if event == model.WebhookEventCommentCreated || event == model.WebhookEventCommentUpdated {
				body["commentId"] = commentID
			}
			got := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", body)
			if got.Status != http.StatusOK {
				t.Fatalf("fire %s: %d %s", event, got.Status, got.Body)
			}
			var fired []map[string]any
			got.JSON(t, &fired)
			if len(fired) != 1 || fired[0]["eventType"] != event || fired[0]["id"] == nil {
				t.Fatalf("fire response must return created delivery ID for %s: %s", event, got.Body)
			}
			after := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhooks/"+webhookID+"/events?perPage=100", nil)
			if after.Status != http.StatusOK {
				t.Fatalf("list events after fire %s: %d %s", event, after.Status, after.Body)
			}
			var afterRows []map[string]any
			after.JSON(t, &afterRows)
			if len(afterRows) != len(beforeRows)+1 {
				t.Fatalf("fire %s persisted %d rows; before=%d after=%d", event, len(afterRows)-len(beforeRows), len(beforeRows), len(afterRows))
			}
			var matching map[string]any
			for _, row := range afterRows {
				id, ok := row["id"].(string)
				if !ok {
					t.Fatalf("persisted event missing id: %v", row)
				}
				if _, existed := beforeIDs[id]; !existed {
					if matching != nil {
						t.Fatalf("fire %s created multiple new rows", event)
					}
					matching = row
				}
			}
			if matching == nil || matching["eventType"] != event {
				t.Fatalf("fire %s created wrong or no event row: %s", event, after.Body)
			}
			if fired[0]["id"] != matching["id"] {
				t.Fatalf("fire %s response ID=%v, persisted ID=%v", event, fired[0]["id"], matching["id"])
			}
			if event != model.WebhookEventPing && matching["ticketId"] != ticketID {
				t.Fatalf("fire %s ticketId=%v, want %s", event, matching["ticketId"], ticketID)
			}
			if event == model.WebhookEventCommentCreated || event == model.WebhookEventCommentUpdated {
				if matching["commentId"] != commentID {
					t.Fatalf("fire %s commentId=%v, want %s", event, matching["commentId"], commentID)
				}
			}
			if attempts := env.DeliverWebhooks(t); attempts != 1 {
				t.Fatalf("fire %s delivery attempts=%d, want 1", event, attempts)
			}
			received := env.Receiver.Requests()
			if len(received) != receiverBaseline+1 {
				t.Fatalf("fire %s receiver requests=%d, want %d", event, len(received), receiverBaseline+1)
			}
			request := received[len(received)-1]
			if !webhook.Verify([]byte(secret), request.Body, request.Header.Get("X-Kickflow-Signature")) {
				t.Fatalf("fire %s request has invalid signature %q", event, request.Header.Get("X-Kickflow-Signature"))
			}
			var payload map[string]any
			if err := json.Unmarshal(request.Body, &payload); err != nil {
				t.Fatalf("decode fire %s payload: %v", event, err)
			}
			if payload["eventType"] != event {
				t.Fatalf("receiver eventType=%v, want %s", payload["eventType"], event)
			}
		})
	}
	for _, event := range []string{"ticket_submitted", "comment_deleted"} {
		t.Run("reject_"+event, func(t *testing.T) {
			body := map[string]any{"eventType": event, "ticketId": ticketID}
			if event == "comment_deleted" {
				body["commentId"] = commentID
			}
			testutil.AssertError(t, env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", body), http.StatusBadRequest, "invalid_parameter")
		})
	}
	missing := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events", map[string]any{"eventType": "comment_created", "ticketId": ticketID})
	testutil.AssertError(t, missing, http.StatusBadRequest, "invalid_parameter")

}

func TestWebhookResendManualAttempt(t *testing.T) {
	env := testutil.New(t)
	const secret = "resend-secret"
	webhookID := env.RegisterReceiverWebhook(t, secret, model.WebhookEventPing)

	ping := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhooks/"+webhookID+"/ping", nil)
	if ping.Status != http.StatusOK {
		t.Fatalf("ping: %d %s", ping.Status, ping.Body)
	}
	var created map[string]any
	ping.JSON(t, &created)
	eventID, _ := created["id"].(string)
	if eventID == "" {
		t.Fatalf("ping response missing event id: %s", ping.Body)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("initial delivery attempts=%d, want 1", attempts)
	}
	first := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhook-events/"+eventID, nil)
	if first.Status != http.StatusOK {
		t.Fatalf("event detail after initial delivery: %d %s", first.Status, first.Body)
	}
	var firstDetail map[string]any
	first.JSON(t, &firstDetail)
	firstAttempts, ok := firstDetail["attempts"].([]any)
	if !ok || len(firstAttempts) != 1 {
		t.Fatalf("initial event attempts: %s", first.Body)
	}
	firstAttempt, ok := firstAttempts[0].(map[string]any)
	if !ok || firstAttempt["manual"] != false || firstDetail["status"] != model.WebhookStatusSucceeded {
		t.Fatalf("initial attempt state: %s", first.Body)
	}

	resend := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events/"+eventID+"/resend", nil)
	if resend.Status != http.StatusOK {
		t.Fatalf("resend: %d %s", resend.Status, resend.Body)
	}
	var resent map[string]any
	resend.JSON(t, &resent)
	if resent["id"] != eventID {
		t.Fatalf("resend id=%v, want original %s", resent["id"], eventID)
	}
	if attempts := env.DeliverWebhooks(t); attempts != 1 {
		t.Fatalf("manual resend attempts=%d, want 1", attempts)
	}
	received := env.Receiver.Requests()
	if len(received) != 2 || !bytes.Equal(received[0].Body, received[1].Body) {
		t.Fatalf("receiver should get two identical bodies, got %d requests", len(received))
	}
	if !webhook.Verify([]byte(secret), received[1].Body, received[1].Header.Get("X-Kickflow-Signature")) {
		t.Fatalf("manual resend has invalid signature %q", received[1].Header.Get("X-Kickflow-Signature"))
	}
	second := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhook-events/"+eventID, nil)
	if second.Status != http.StatusOK {
		t.Fatalf("event detail after resend: %d %s", second.Status, second.Body)
	}
	var secondDetail map[string]any
	second.JSON(t, &secondDetail)
	secondAttempts, ok := secondDetail["attempts"].([]any)
	if !ok || len(secondAttempts) != 2 || secondDetail["id"] != eventID || secondDetail["status"] != model.WebhookStatusSucceeded {
		t.Fatalf("resent event state: %s", second.Body)
	}
	lastAttempt, ok := secondAttempts[1].(map[string]any)
	if !ok || lastAttempt["manual"] != true {
		t.Fatalf("last attempt should be manual: %s", second.Body)
	}
	testutil.AssertError(t, env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhook-events/unknown-event/resend", nil), http.StatusNotFound, "resource_not_found")
}
func TestWebhookEventPaginationAndDetails(t *testing.T) {
	env := testutil.New(t)
	id := pf24TestHook(t, env, model.WebhookEventPing)
	created := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhooks/"+id+"/ping", nil)
	if created.Status != http.StatusOK {
		t.Fatalf("ping: %d %s", created.Status, created.Body)
	}
	page := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhooks/"+id+"/events?page=1&perPage=10", nil)
	if page.Status != http.StatusOK {
		t.Fatalf("events: %d %s", page.Status, page.Body)
	}
	if page.Header.Get("Total") != "1" {
		t.Fatalf("Total=%q", page.Header.Get("Total"))
	}
	var rows []map[string]any
	page.JSON(t, &rows)
	eventID := rows[0]["id"].(string)
	detail := env.Do(t, testutil.Integration, http.MethodGet, "/admin/api/webhook-events/"+eventID, nil)
	if detail.Status != http.StatusOK {
		t.Fatalf("detail: %d %s", detail.Status, detail.Body)
	}
	var event map[string]any
	detail.JSON(t, &event)
	if event["body"] == nil || event["attempts"] == nil {
		t.Fatalf("detail missing body/attempts: %s", detail.Body)
	}
}

func pf24TestHook(t *testing.T, env *testutil.Env, events ...string) string {
	t.Helper()
	id := store.NewID()
	secret := "secret"
	now := time.Now()
	if err := store.InsertWebhook(t.Context(), env.DB.Q(), model.Webhook{ID: id, Name: "test", URL: "http://127.0.0.1:1/hook", Secret: &secret, Enabled: true, WorkflowIDs: []string{}, EventTypes: events, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}
