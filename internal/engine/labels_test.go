package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf15LabelID(t *testing.T, e *pf14TestEnv, name string) string {
	t.Helper()
	labels, err := store.Labels(e.ctx, e.db.Q())
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range labels {
		if label.Name == name {
			return label.ID
		}
	}
	t.Fatalf("no label named %q", name)
	return ""
}

func pf15LabelNames(t *testing.T, e *pf14TestEnv, id string) []string {
	t.Helper()
	labels, err := store.TicketLabels(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names
}

func TestAttachAndDetachLabels(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim", pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	urgent, reviewed := pf15LabelID(t, e, "Urgent"), pf15LabelID(t, e, "Reviewed")
	attach := func(actor, labelID string) ([]model.WebhookEvent, error) {
		return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.AttachLabel(e.ctx, tx, e.actor(actor), id, labelID)
		})
	}
	detach := func(actor, labelID string) ([]model.WebhookEvent, error) {
		return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.DetachLabel(e.ctx, tx, e.actor(actor), id, labelID)
		})
	}
	events, err := attach("sales-rep", urgent)
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "attach", events, model.WebhookEventTicketUpdated)
	// Any viewer may label the ticket, not only the applicant.
	if _, err = attach("sales-manager", reviewed); err != nil {
		t.Fatalf("approver attach: %v", err)
	}
	if got := pf15LabelNames(t, e, id); len(got) != 2 || got[0] != "Reviewed" || got[1] != "Urgent" {
		t.Fatalf("labels=%v, want [Reviewed Urgent]", got)
	}
	events, err = attach("sales-rep", urgent)
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "repeated attach", events)
	events, err = detach("sales-rep", urgent)
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "detach", events, model.WebhookEventTicketUpdated)
	events, err = detach("sales-rep", urgent)
	if err != nil {
		t.Fatal(err)
	}
	compatAssertTypes(t, "repeated detach", events)
	if got := pf15LabelNames(t, e, id); len(got) != 1 || got[0] != "Reviewed" {
		t.Fatalf("labels after detach=%v, want [Reviewed]", got)
	}
	_, err = attach("sales-rep", "00000000-0000-4000-8000-00000000ffff")
	pf15WantCode(t, "unknown label", err, "validation_failed")
	_, err = attach("hr-manager", urgent)
	pf15WantCode(t, "invisible ticket", err, "invisible_ticket")
}
