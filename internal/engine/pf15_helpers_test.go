package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf15RouteStep builds a static user route step with a sub-status and raise flag.
func pf15RouteStep(code string, order int, action, subStatus string, raising bool, users ...string) model.RouteStep {
	step := compatUserStep(code, order, action, users...)
	step.AllowRaising = raising
	if subStatus != "" {
		step.SubStatus = compatStr(subStatus)
	}
	return step
}

// pf15Run runs one engine operation in a transaction.
func pf15Run(e *pf14TestEnv, op func(tx *store.Tx) ([]model.WebhookEvent, error)) ([]model.WebhookEvent, error) {
	var events []model.WebhookEvent
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = op(tx)
		return err
	})
	return events, err
}

func pf15WantCode(t *testing.T, label string, err error, code string) {
	t.Helper()
	api, ok := kferr.As(err)
	if !ok || api.Code != code {
		t.Fatalf("%s error=%v, want code %s", label, err, code)
	}
}

// pf15SubStatusCode returns the code of the sub-status attached to the ticket.
func pf15SubStatusCode(t *testing.T, e *pf14TestEnv, id string) string {
	t.Helper()
	attached, err := store.TicketSubStatusID(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if attached == nil {
		return ""
	}
	item, err := store.SubStatusByID(e.ctx, e.db.Q(), *attached)
	if err != nil {
		t.Fatal(err)
	}
	return item.Code
}

// pf15Steps loads the ticket steps keyed by display order.
func pf15Steps(t *testing.T, e *pf14TestEnv, id string) []model.TicketStep {
	t.Helper()
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func pf15StepByTitle(t *testing.T, e *pf14TestEnv, id, title string) model.TicketStep {
	t.Helper()
	for _, step := range pf15Steps(t, e, id) {
		if step.Title != nil && *step.Title == title {
			return step
		}
	}
	t.Fatalf("ticket %s has no step %q", id, title)
	return model.TicketStep{}
}

func pf15Withdraw(e *pf14TestEnv, actor, id string) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.Withdraw(e.ctx, tx, e.actor(actor), id)
	})
}

func pf15Deny(e *pf14TestEnv, actor, id string) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.Deny(e.ctx, tx, e.actor(actor), id, nil)
	})
}

func pf15SendBack(e *pf14TestEnv, actor, id string, to int) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.SendBack(e.ctx, tx, e.actor(actor), id, to, nil)
	})
}

func pf15Approve(e *pf14TestEnv, actor, id string) ([]model.WebhookEvent, error) {
	return pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
		return e.engine.Approve(e.ctx, tx, e.actor(actor), id, nil)
	})
}

func pf15Controls(t *testing.T, e *pf14TestEnv, actor, id string) TicketControls {
	t.Helper()
	controls, err := e.engine.Controls(e.ctx, e.db.Q(), e.actor(actor), id)
	if err != nil {
		t.Fatal(err)
	}
	return controls
}
