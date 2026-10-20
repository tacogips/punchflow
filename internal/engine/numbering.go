package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14Number(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time) error {
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
	format := *def.TicketNumberFormat
	number := strings.NewReplacer("[year]", now.In(time.FixedZone("JST", 9*60*60)).Format("2006"), "[number]", strconv.FormatInt(counter, 10)).Replace(format)
	ticket.TicketNumber = &number
	return nil
}
