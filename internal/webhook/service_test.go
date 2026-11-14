package webhook

import (
	"context"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type pf09TestBuilder struct{ body []byte }

func (b pf09TestBuilder) Build(context.Context, store.Querier, model.WebhookEvent) ([]byte, error) {
	return append([]byte(nil), b.body...), nil
}

func pf09TestDB(t *testing.T) (*store.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/test.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close DB: %v", err)
		}
	})
	return db, ctx
}
func pf09InsertWebhook(t *testing.T, ctx context.Context, db *store.DB, w model.Webhook) {
	t.Helper()
	if err := store.InsertWebhook(ctx, db.Q(), w); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookFanOutFilters(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	svc := New(db, pf09TestBuilder{[]byte(`{"eventType":"ticket_created"}`)}, clock, nil, Config{})
	_, err := db.Q().ExecContext(ctx, "INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f','f','folder',?,?)", store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Q().ExecContext(ctx, "INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w','w','active','f','{}',?,?)", store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Q().ExecContext(ctx, "INSERT INTO tickets(id,workflow_id,status,created_at,updated_at) VALUES('t','w','draft',?,?)", store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "match", Name: "match", URL: "http://localhost/hook", Enabled: true, EventTypes: []string{"ticket_created"}, WorkflowIDs: []string{"w"}})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "wrong-event", Name: "wrong", URL: "http://localhost/hook", Enabled: true, EventTypes: []string{"comment_created"}})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "wrong-flow", Name: "wrong", URL: "http://localhost/hook", Enabled: true, WorkflowIDs: []string{"other"}})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "disabled", Name: "disabled", URL: "http://localhost/hook", Enabled: false})
	if err := svc.Enqueue(ctx, db.Q(), []model.WebhookEvent{{Type: "ping"}, {Type: "ticket_created", TicketID: "t"}}); err != nil {
		t.Fatal(err)
	}
	rows, total, err := store.ListWebhookEvents(ctx, db.Q(), "match", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("matching deliveries = %d, want 1", total)
	}
	for _, id := range []string{"wrong-event", "wrong-flow", "disabled"} {
		_, count, err := store.ListWebhookEvents(ctx, db.Q(), id, 0, 20)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s got %d deliveries", id, count)
		}
	}
	ids, err := svc.EnqueueTo(ctx, db.Q(), model.WebhookEvent{Type: "ping"}, []string{"disabled"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("EnqueueTo ids = %d, want 1", len(ids))
	}
	if _, err := store.GetWebhookEvent(ctx, db.Q(), "missing"); err != store.ErrNotFound {
		t.Errorf("missing event error = %v, want ErrNotFound", err)
	}
}
