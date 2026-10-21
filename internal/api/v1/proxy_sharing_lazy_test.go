package v1_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestFutureProxyRegistrationMaterializesOnFirstEffectiveRequest(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	env.Clock.Set(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)))
	workflow, err := store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	principalID := env.UserID(t, "sales-manager")
	proxyID := store.NewID()
	ticketID, stepID := store.NewID(), store.NewID()
	now := env.Clock.Now()
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,'future-proxy','future-proxy@example.test','Future','Proxy','activated','en','member','{}',?,?)`, proxyID, store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO tickets(id,workflow_id,status,author_id,created_at,updated_at) VALUES(?,?,?,?,?,?)`, ticketID, workflow.ID, model.TicketStatusInProgress, env.UserID(t, "sales-rep"), store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO ticket_steps(id,ticket_id,step_order,display_order,action_type,status,created_at,updated_at) VALUES(?,?,1,1,'approve','waiting',?,?)`, stepID, ticketID, store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,created_at,updated_at) VALUES(?,?,?,?,?,?)`, store.NewID(), stepID, ticketID, principalID, store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO proxy_approvers(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES(?,?,?,'2026-01-02',NULL,'[]',?,?)`, store.NewID(), principalID, proxyID, store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	token := "future-proxy-test-token"
	if _, err := env.DB.Q().ExecContext(ctx, `INSERT INTO api_tokens(id,kind,name,token_hash,user_id,created_at) VALUES(?, 'pat','future proxy test',?,?,?)`, store.NewID(), auth.HashToken(token), proxyID, store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTicketWatchState(ctx, env.DB.Q(), ticketID, proxyID, store.WatchStateUnwatched, now); err != nil {
		t.Fatal(err)
	}
	path := "/v1/tickets/" + ticketID
	request := func() *testutil.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return env.DoRaw(t, req)
	}
	denied := request()
	if denied.Status != http.StatusForbidden {
		t.Fatalf("proxy request before start: status=%d body=%s", denied.Status, denied.Body)
	}
	env.Clock.Set(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)))
	allowed := request()
	if allowed.Status != http.StatusOK {
		t.Fatalf("proxy request after start: status=%d body=%s", allowed.Status, allowed.Body)
	}
	viewers, err := store.TicketViewers(ctx, env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	shared := false
	for _, viewer := range viewers {
		shared = shared || viewer.UserID != nil && *viewer.UserID == proxyID
	}
	if !shared {
		t.Fatal("effective proxy registration was not materialized as a ticket viewer")
	}
	states, err := store.TicketWatchStates(ctx, env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if states[proxyID] != store.WatchStateUnwatched {
		t.Fatalf("proxy watch choice=%q, want preserved explicit opt-out", states[proxyID])
	}
}
