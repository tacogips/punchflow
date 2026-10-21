package admin_test

import (
	"net/http"
	"testing"
	"time"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

// WH-05: ordinary edits of a failing enabled webhook must not restart the
// continuous-failure clock; only a disabled-to-enabled transition does.
func TestWebhookEditPreservesFailureHistory(t *testing.T) {
	env := testutil.New(t)
	id := env.RegisterReceiverWebhook(t, "secret", "ticket_created")
	env.Receiver.RespondWith(http.StatusInternalServerError)
	if ping := env.Do(t, testutil.Integration, http.MethodPost, "/admin/api/webhooks/"+id+"/ping", nil); ping.Status != http.StatusOK {
		t.Fatalf("ping: %d %s", ping.Status, ping.Body)
	}
	firstFailure := env.Clock.Now()
	if n := env.DeliverWebhooks(t); n != 1 {
		t.Fatalf("first delivery attempts=%d, want 1", n)
	}
	hook, err := store.GetWebhook(t.Context(), env.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if hook.FirstFailureAt == nil || !hook.FirstFailureAt.Equal(firstFailure) {
		t.Fatalf("first_failure_at after first failure = %v, want %v", hook.FirstFailureAt, firstFailure)
	}

	env.Clock.Advance(6 * 24 * time.Hour)
	for _, patch := range []map[string]any{
		{"name": "renamed"},
		{"eventTypes": []string{"ticket_created", "ticket_updated"}},
		{"workflowIds": []string{}},
		{"enabled": true},
	} {
		if res := env.Do(t, testutil.Integration, http.MethodPatch, "/admin/api/webhooks/"+id, patch); res.Status != http.StatusOK {
			t.Fatalf("patch %v: %d %s", patch, res.Status, res.Body)
		}
		hook, err = store.GetWebhook(t.Context(), env.DB.Q(), id)
		if err != nil {
			t.Fatal(err)
		}
		if hook.FirstFailureAt == nil || !hook.FirstFailureAt.Equal(firstFailure) {
			t.Fatalf("patch %v reset first_failure_at to %v, want %v", patch, hook.FirstFailureAt, firstFailure)
		}
	}

	// One uninterrupted week of failures after the edits still disables the hook.
	env.Clock.Advance(24 * time.Hour)
	if n := env.DeliverWebhooks(t); n != 1 {
		t.Fatalf("retry attempts=%d, want 1", n)
	}
	hook, err = store.GetWebhook(t.Context(), env.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if hook.Enabled || hook.DisabledAt == nil {
		t.Fatalf("webhook should be auto-disabled after a week of failures: %#v", hook)
	}

	// An ordinary edit of the disabled webhook keeps its history; re-enabling clears it.
	if res := env.Do(t, testutil.Integration, http.MethodPatch, "/admin/api/webhooks/"+id, map[string]any{"name": "still disabled"}); res.Status != http.StatusOK {
		t.Fatalf("patch disabled: %d %s", res.Status, res.Body)
	}
	hook, err = store.GetWebhook(t.Context(), env.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if hook.Enabled || hook.FirstFailureAt == nil {
		t.Fatalf("editing a disabled webhook must not re-enable or clear history: %#v", hook)
	}
	if res := env.Do(t, testutil.Integration, http.MethodPatch, "/admin/api/webhooks/"+id, map[string]any{"enabled": true}); res.Status != http.StatusOK {
		t.Fatalf("re-enable: %d %s", res.Status, res.Body)
	}
	hook, err = store.GetWebhook(t.Context(), env.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !hook.Enabled || hook.FirstFailureAt != nil || hook.DisabledAt != nil {
		t.Fatalf("re-enable should clear failure state: %#v", hook)
	}
}
