package engine

import (
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
)

func compatNumber(t *testing.T, e *pf14TestEnv, id string) string {
	t.Helper()
	if n := e.ticket(id).TicketNumber; n != nil {
		return *n
	}
	return ""
}

// PF-13: the numbering timing selects when the number is allocated (INF-35).
func TestTicketNumberingTimings(t *testing.T) {
	newEnv := func(timing string) *pf14TestEnv {
		e := pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberingTiming", timing)
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberFormat", "N-[number]")
		return e
	}

	t.Run("created", func(t *testing.T) {
		e := newEnv("created")
		draft, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		if got := compatNumber(t, e, draft); got != "N-1" {
			t.Fatalf("draft number=%q, want N-1 at creation", got)
		}
		if _, err := compatUpdate(e, "sales-rep", draft, UpdateInput{Status: compatStr(model.TicketStatusInProgress)}); err != nil {
			t.Fatal(err)
		}
		if got := compatNumber(t, e, draft); got != "N-1" {
			t.Fatalf("number changed on submission: %q", got)
		}
		direct, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, direct); got != "N-2" {
			t.Fatalf("direct create number=%q, want N-2", got)
		}
	})

	for _, timing := range []string{"opened", "sub_status_attached"} {
		t.Run(timing, func(t *testing.T) {
			e := newEnv(timing)
			draft, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
			if got := compatNumber(t, e, draft); got != "" {
				t.Fatalf("draft number=%q, want none before submission", got)
			}
			if _, err := compatUpdate(e, "sales-rep", draft, UpdateInput{Status: compatStr(model.TicketStatusInProgress)}); err != nil {
				t.Fatal(err)
			}
			if got := compatNumber(t, e, draft); got != "N-1" {
				t.Fatalf("number after submission=%q, want N-1", got)
			}
		})
	}

	t.Run("completed", func(t *testing.T) {
		e := newEnv("completed")
		compatSetRoute(t, e, "expense-claim", compatUserStep("only", 1, model.ActionTypeApprove, "sales-manager"))
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, id); got != "" {
			t.Fatalf("number while in progress=%q, want none", got)
		}
		compatApprove(t, e, "sales-manager", id)
		if got := compatNumber(t, e, id); got != "N-1" {
			t.Fatalf("number on completion=%q, want N-1", got)
		}
	})

	t.Run("completed on submission", func(t *testing.T) {
		e := newEnv("completed")
		compatSetRoute(t, e, "expense-claim", compatUserStep("fyi", 1, model.ActionTypeNone, "sales-manager"))
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, id); got != "N-1" {
			t.Fatalf("number of immediately completed ticket=%q, want N-1", got)
		}
	})
}

// PF-13: documented format tokens, in JST, with a fiscal year starting in April.
func TestTicketNumberFormatTokens(t *testing.T) {
	e := pf14Setup(t)
	compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberFormat", "X-[fyear]-[year][month][day]-[number]")
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2025, 2, 3, 10, 0, 0, 0, kftime.JST), "X-2024-20250203-1"},
		{time.Date(2025, 4, 1, 0, 30, 0, 0, kftime.JST), "X-2025-20250401-2"},
		{time.Date(2025, 12, 31, 23, 0, 0, 0, kftime.JST), "X-2025-20251231-3"},
		// 2025-03-31T16:00Z is 2025-04-01 01:00 JST: the date is read in JST.
		{time.Date(2025, 3, 31, 16, 0, 0, 0, time.UTC), "X-2025-20250401-4"},
	}
	for _, tc := range cases {
		e.clock.Set(tc.at)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, id); got != tc.want {
			t.Errorf("at %s: ticket number=%q, want %q", tc.at.Format(time.RFC3339), got, tc.want)
		}
	}
}
