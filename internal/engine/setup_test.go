package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"punchflow"
	"punchflow/internal/fixtures"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type pf14TestEnv struct {
	t      *testing.T
	ctx    context.Context
	db     *store.DB
	engine *Engine
	clock  *kftime.FakeClock
}

func pf14Setup(t *testing.T) *pf14TestEnv {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fixtureFS, err := fixtures.DefaultFS(punchflow.DefaultFixtures)
	if err != nil {
		t.Fatal(err)
	}
	clock := kftime.NewFakeClock(time.Date(2025, 2, 3, 10, 0, 0, 0, kftime.JST))
	if _, err = fixtures.Load(ctx, db, fixtureFS, clock); err != nil {
		t.Fatal(err)
	}
	return &pf14TestEnv{t: t, ctx: ctx, db: db, engine: New(clock), clock: clock}
}
func (e *pf14TestEnv) user(code string) string {
	e.t.Helper()
	id, err := store.UserIDByCode(e.ctx, e.db.Q(), code)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}
func (e *pf14TestEnv) team(code string) string {
	e.t.Helper()
	chart, err := store.ActiveOrgChartID(e.ctx, e.db.Q())
	if err != nil {
		e.t.Fatal(err)
	}
	id, err := store.TeamIDByCode(e.ctx, e.db.Q(), chart, code)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}
func (e *pf14TestEnv) createExpense(actorCode string, amount int, submit bool) string {
	e.t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		e.t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	fields := map[string]string{}
	for _, s := range def.SectionList {
		for _, f := range s.FormFields {
			fields[f.Code] = f.ID
		}
	}
	value, _ := json.Marshal(amount)
	purpose, _ := json.Marshal("client meal")
	status := model.TicketStatusDraft
	if submit {
		status = model.TicketStatusInProgress
	}
	actor := Actor{UserID: e.user(actorCode)}
	var id string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, actor, CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &[]FieldInput{{FormFieldID: fields["amount"], FormFieldCode: "amount", Value: value}, {FormFieldID: fields["purpose"], FormFieldCode: "purpose", Value: purpose}}})
		return err
	})
	if err != nil {
		e.t.Fatalf("create expense: %v", err)
	}
	return id
}
func (e *pf14TestEnv) actor(code string) Actor { return Actor{UserID: e.user(code)} }
func (e *pf14TestEnv) ticket(id string) model.Ticket {
	e.t.Helper()
	x, err := store.EngLoadTicket(e.ctx, e.db.Q(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return x
}
func (e *pf14TestEnv) apply(fn func(*store.Tx) error) {
	e.t.Helper()
	if err := e.db.WithTx(e.ctx, fn); err != nil {
		e.t.Fatal(err)
	}
}
