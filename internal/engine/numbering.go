package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Numbering events. A workflow's ticketNumberingTiming selects the single event
// at which the ticket number is allocated (INF-35).
const (
	numberingEventCreated   = "created"
	numberingEventOpened    = "opened"
	numberingEventCompleted = "completed"

	numberingTimingSubStatusAttached = "sub_status_attached"
)

// pf14NumberingDue reports whether a workflow configured with timing allocates
// its ticket number at event. An empty timing defaults to "opened", and
// "sub_status_attached" behaves like "opened" because sub-statuses are not
// modelled.
func pf14NumberingDue(timing, event string) bool {
	switch timing {
	case "", numberingTimingSubStatusAttached:
		timing = numberingEventOpened
	}
	return timing == event
}

// pf14Number allocates the ticket number when the workflow numbers tickets at
// the given event. It is idempotent: a ticket that already has a number keeps it.
func pf14Number(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time, event string) error {
	if !pf14NumberingDue(def.TicketNumberingTiming, event) {
		return nil
	}
	if ticket.TicketNumber != nil && *ticket.TicketNumber != "" {
		return nil
	}
	if def.TicketNumberFormat == nil || *def.TicketNumberFormat == "" {
		return nil
	}
	var counter int64
	if err := tx.QueryRowContext(ctx, `SELECT ticket_number_counter FROM workflows WHERE id=?`, ticket.WorkflowID).Scan(&counter); err != nil {
		return fmt.Errorf("engine: read ticket number counter: %w", err)
	}
	counter++
	if _, err := tx.ExecContext(ctx, `UPDATE workflows SET ticket_number_counter=?,updated_at=? WHERE id=?`, counter, store.Millis(now), ticket.WorkflowID); err != nil {
		return fmt.Errorf("engine: increment ticket number counter: %w", err)
	}
	number := pf14FormatTicketNumber(*def.TicketNumberFormat, now, counter)
	ticket.TicketNumber = &number
	return nil
}

// pf14FormatTicketNumber expands the documented format tokens in JST:
// [year], [month] and [day] (zero padded to 4, 2 and 2 digits), [fyear] (the
// fiscal year, which starts in April and is named after its starting calendar
// year) and [number] (the per-workflow counter, not padded).
func pf14FormatTicketNumber(format string, now time.Time, counter int64) string {
	local := now.In(kftime.JST)
	fiscalYear := local.Year()
	if local.Month() < time.April {
		fiscalYear--
	}
	return strings.NewReplacer(
		"[year]", fmt.Sprintf("%04d", local.Year()),
		"[month]", fmt.Sprintf("%02d", int(local.Month())),
		"[day]", fmt.Sprintf("%02d", local.Day()),
		"[fyear]", fmt.Sprintf("%04d", fiscalYear),
		"[number]", strconv.FormatInt(counter, 10),
	).Replace(format)
}
