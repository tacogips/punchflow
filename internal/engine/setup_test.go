package engine

import (
	"context"
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
	users  map[string]string
	teams  map[string]string
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
	users := map[string]string{}
	rows, err := db.Q().QueryContext(ctx, `SELECT id, code FROM users`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		users[code] = id
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	chart, err := store.ActiveOrgChartID(ctx, db.Q())
	if err != nil {
		t.Fatal(err)
	}
	teams := map[string]string{}
	rows, err = db.Q().QueryContext(ctx, `SELECT id, code FROM teams WHERE organization_chart_id=?`, chart)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		teams[code] = id
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	return &pf14TestEnv{t: t, ctx: ctx, db: db, engine: New(clock), clock: clock, users: users, teams: teams}
}
func (e *pf14TestEnv) user(code string) string {
	e.t.Helper()
	id, ok := e.users[code]
	if !ok {
		e.t.Fatalf("unknown test user code %q", code)
	}
	return id
}
func (e *pf14TestEnv) team(code string) string {
	e.t.Helper()
	id, ok := e.teams[code]
	if !ok {
		e.t.Fatalf("unknown test team code %q", code)
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
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": amount, "purpose": "client meal"})
	status := model.TicketStatusDraft
	if submit {
		status = model.TicketStatusInProgress
	}
	actor := Actor{UserID: e.user(actorCode)}
	teamID := e.team("sales")
	var id string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, actor, CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: teamID, Inputs: &inputs})
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
