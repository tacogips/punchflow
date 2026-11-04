package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"punchflow/internal/testutil"
	"punchflow/internal/webhook"
)

func TestTicketWebhookLifecycleEventsAndSignatures(t *testing.T) {
	env := testutil.New(t)
	secret := "s3cret"
	env.RegisterReceiverWebhook(t, secret)

	updatedID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	updated := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+updatedID, map[string]any{"status": "in_progress", "title": "Opened from draft"})
	if updated.Status != http.StatusOK {
		t.Fatalf("submit draft: %d %s", updated.Status, updated.Body)
	}

	largeID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 200000)
	for _, persona := range []testutil.Persona{testutil.Manager, testutil.Finance} {
		res := env.Do(t, persona, http.MethodPost, "/v1/tickets/"+largeID+"/approve", nil)
		if res.Status != http.StatusOK {
			t.Fatalf("approve as %s: %d %s", persona, res.Status, res.Body)
		}
	}
	env.CallerFor = "ceo"
	if res := env.Do(t, testutil.Integration, http.MethodPost, "/v1/tickets/"+largeID+"/approve", nil); res.Status != http.StatusOK {
		t.Fatalf("CEO confirm: %d %s", res.Status, res.Body)
	}
	env.CallerFor = "accountant"
	completed := env.Do(t, testutil.Integration, http.MethodPost, "/v1/tickets/"+largeID+"/approve", nil)
	if completed.Status != http.StatusOK {
		t.Fatalf("accountant confirm: %d %s", completed.Status, completed.Body)
	}
	archived := env.Do(t, testutil.Admin, http.MethodPost, "/v1/tickets/"+largeID+"/archive", nil)
	if archived.Status != http.StatusOK {
		t.Fatalf("archive: %d %s", archived.Status, archived.Body)
	}

	rejectedID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejectedID+"/reject", map[string]any{"to": 0}); res.Status != http.StatusOK {
		t.Fatalf("reject: %d %s", res.Status, res.Body)
	}
	deniedID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+deniedID+"/deny", nil); res.Status != http.StatusOK {
		t.Fatalf("deny: %d %s", res.Status, res.Body)
	}
	withdrawID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	if res := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+withdrawID+"/withdraw", nil); res.Status != http.StatusOK {
		t.Fatalf("withdraw: %d %s", res.Status, res.Body)
	}

	if attempted := env.DeliverWebhooks(t); attempted < 10 {
		t.Fatalf("delivery attempts=%d, want at least 10", attempted)
	}
	received := env.Receiver.Requests()
	want := map[string]bool{"ticket_created": true, "ticket_updated": true, "ticket_opened": true, "ticket_approved": true, "ticket_confirmed": true, "ticket_rejected": true, "ticket_denied": true, "ticket_completed": true, "ticket_withdrawn": true, "ticket_archived": true}
	for _, request := range received {
		var payload struct {
			EventType string `json:"eventType"`
			Data      struct {
				Ticket struct {
					Status string `json:"status"`
				} `json:"ticket"`
			} `json:"data"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			t.Fatalf("decode webhook: %v", err)
		}
		if _, ok := want[payload.EventType]; ok {
			want[payload.EventType] = false
		}
		if payload.EventType == "ticket_completed" && payload.Data.Ticket.Status != "completed" {
			t.Errorf("ticket_completed payload status=%q, want completed", payload.Data.Ticket.Status)
		}
		if !webhook.Verify([]byte(secret), request.Body, request.Header.Get("X-Kickflow-Signature")) {
			t.Errorf("invalid signature for %s", payload.EventType)
		}
		if got := request.Header.Get("User-Agent"); got != "kickflow-Hookshot/v1" {
			t.Errorf("User-Agent=%q", got)
		}
	}
	for eventType, missing := range want {
		if missing {
			t.Errorf("receiver did not observe %s", eventType)
		}
	}
	var events, attempts int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_events WHERE event_type LIKE 'ticket_%'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_attempts wa JOIN webhook_events we ON we.id=wa.event_id WHERE we.event_type LIKE 'ticket_%'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if events < 10 || attempts < 10 {
		t.Fatalf("persisted ticket events/attempts=%d/%d", events, attempts)
	}
}

func TestTicketWebhookRetryKeepsDeliveryID(t *testing.T) {
	env := testutil.New(t)
	env.Receiver.RespondWith(http.StatusInternalServerError, http.StatusOK)
	env.RegisterReceiverWebhook(t, "retry-secret", "ticket_created")
	pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	if n := env.DeliverWebhooks(t); n != 1 {
		t.Fatalf("first delivery attempts=%d", n)
	}
	first := env.Receiver.Requests()
	if len(first) != 1 {
		t.Fatalf("first receiver requests=%d", len(first))
	}
	var delivery struct {
		ID string `json:"id"`
	}
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT id FROM webhook_events WHERE event_type='ticket_created'`).Scan(&delivery.ID); err != nil {
		t.Fatal(err)
	}
	env.Clock.Advance(30 * time.Second)
	if n := env.DeliverWebhooks(t); n != 1 {
		t.Fatalf("retry attempts=%d", n)
	}
	requests := env.Receiver.Requests()
	if len(requests) != 2 {
		t.Fatalf("receiver requests=%d", len(requests))
	}
	for i, request := range requests {
		if got := request.Header.Get("X-Kickflow-Delivery"); got != delivery.ID {
			t.Errorf("attempt %d delivery ID=%q want %q", i+1, got, delivery.ID)
		}
		if !webhook.Verify([]byte("retry-secret"), request.Body, request.Header.Get("X-Kickflow-Signature")) {
			t.Errorf("attempt %d signature invalid", i+1)
		}
	}
}
