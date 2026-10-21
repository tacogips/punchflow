package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestTicketNumberGroupSharesCounterAcrossWorkflows(t *testing.T) {
	e := pf14Setup(t)
	key := &model.TicketNumberKey{ID: "4f768c52-44de-4e4a-924b-693fbb3bffb8", Code: "SHARED", Name: "Shared", Default: false}
	defs := make([]model.WorkflowDef, 0, 2)
	for _, code := range []string{"expense-claim", "leave-request"} {
		workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), code)
		if err != nil {
			t.Fatal(err)
		}
		def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
		if err != nil {
			t.Fatal(err)
		}
		def.TicketNumberKey = key
		def.TicketNumberFormat = ptrNumbering("G-[number]")
		def.TicketNumberingTiming = numberingEventCreated
		defs = append(defs, def)
	}
	for i, def := range defs {
		ticket := model.Ticket{ID: "ticket-" + string(rune('1'+i)), WorkflowID: def.ID}
		if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			return pf14Number(e.ctx, tx, &ticket, def, e.clock.Now(), numberingEventCreated)
		}); err != nil {
			t.Fatal(err)
		}
		want := "G-" + string(rune('1'+i))
		if ticket.TicketNumber == nil || *ticket.TicketNumber != want {
			t.Fatalf("workflow %s ticket number=%v, want %q", def.Code, ticket.TicketNumber, want)
		}
	}
	var counter int64
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT counter FROM ticket_number_key_counters WHERE ticket_number_key_id=?`, key.ID).Scan(&counter); err != nil || counter != 2 {
		t.Fatalf("shared counter=%d err=%v, want 2", counter, err)
	}
}

func TestCustomNumberingTimingAndPersistedValue(t *testing.T) {
	e := pf14Setup(t)
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	id := "124a3c20-74e3-4a73-9466-1a45375486b1"
	definition := model.CustomNumbering{ID: id, Code: "DECISION", Name: "Decision number", Value: 9}
	format := "D-[number]"
	def := model.WorkflowDef{ID: workflow.ID, TicketNumberingTiming: "created", CustomNumberings: []model.CustomNumbering{definition}, CustomNumberingSettings: []model.CustomNumberingSetting{{ID: "224a3c20-74e3-4a73-9466-1a45375486b1", CustomNumberingID: id, TicketNumberFormat: &format, TimingType: numberingEventOpened}}}
	ticketID := "custom-numbering-ticket"
	if _, err := e.db.Q().ExecContext(e.ctx, `INSERT INTO tickets(id,workflow_id,status,current_step,created_at,updated_at) VALUES(?,?,'draft',0,?,?)`, ticketID, workflow.ID, store.Millis(e.clock.Now()), store.Millis(e.clock.Now())); err != nil {
		t.Fatal(err)
	}
	ticket := model.Ticket{ID: ticketID, WorkflowID: workflow.ID}
	for _, event := range []string{numberingEventCreated, numberingEventOpened, numberingEventOpened} {
		if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			return pf14Number(e.ctx, tx, &ticket, def, e.clock.Now(), event)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var formatted string
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, ticketID, id).Scan(&formatted); err != nil || formatted != "D-9" {
		t.Fatalf("stored custom number=%q err=%v, want D-9", formatted, err)
	}
	var counter int64
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT counter FROM custom_numbering_counters WHERE custom_numbering_id=?`, "custom:"+id).Scan(&counter); err != nil || counter != 9 {
		t.Fatalf("custom counter=%d err=%v, want 9", counter, err)
	}
}

func ptrNumbering(value string) *string { return &value }
