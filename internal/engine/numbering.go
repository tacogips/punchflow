package engine

import (
	"context"
	"database/sql"
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
	// numberingEventSubStatusAttached fires when a sub-status is attached (INF-35).
	numberingEventSubStatusAttached = model.TicketNumberingTimingSubStatusAttached
)

// pf14NumberingDue reports whether a workflow configured with timing allocates
// its ticket number at event. An empty timing defaults to "opened".
func pf14NumberingDue(timing, event string) bool {
	if timing == "" {
		timing = numberingEventOpened
	}
	return timing == event
}

// pf14Number allocates the ticket number when the workflow numbers tickets at
// the given event. It is idempotent: a ticket that already has a number keeps it.
func pf14Number(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time, event string) error {
	tenant, err := store.Tenant(ctx, tx)
	if err != nil {
		return err
	}
	baseDue := pf14NumberingDue(def.TicketNumberingTiming, event)
	if baseDue && event == numberingEventSubStatusAttached && def.TicketNumberingSubStatus != nil {
		activeID, err := store.TicketSubStatusID(ctx, tx, ticket.ID)
		if err != nil {
			return err
		}
		configured, err := store.SubStatusByCode(ctx, tx, *def.TicketNumberingSubStatus)
		if err != nil || activeID == nil || *activeID != configured.ID {
			baseDue = false
		}
	}
	if baseDue && (ticket.TicketNumber == nil || *ticket.TicketNumber == "") && def.TicketNumberFormat != nil && *def.TicketNumberFormat != "" {
		var counter int64
		if def.TicketNumberKey != nil && def.TicketNumberKey.ID != "" {
			if err := tx.QueryRowContext(ctx, `INSERT INTO ticket_number_key_counters(ticket_number_key_id,counter) VALUES(?,1) ON CONFLICT(ticket_number_key_id) DO UPDATE SET counter=counter+1 RETURNING counter`, def.TicketNumberKey.ID).Scan(&counter); err != nil {
				return fmt.Errorf("engine: increment ticket number group counter: %w", err)
			}
		} else {
			if err := tx.QueryRowContext(ctx, `SELECT ticket_number_counter FROM workflows WHERE id=?`, ticket.WorkflowID).Scan(&counter); err != nil {
				return fmt.Errorf("engine: read ticket number counter: %w", err)
			}
			counter++
			if _, err := tx.ExecContext(ctx, `UPDATE workflows SET ticket_number_counter=?,updated_at=? WHERE id=?`, counter, store.Millis(now), ticket.WorkflowID); err != nil {
				return fmt.Errorf("engine: increment ticket number counter: %w", err)
			}
		}
		number := pf14FormatTicketNumber(*def.TicketNumberFormat, now, counter, tenant.FiscalYearStartMonth)
		ticket.TicketNumber = &number
	}
	return pf14CustomNumbers(ctx, tx, ticket, def, now, event)
}

func pf14CustomNumbers(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time, event string) error {
	tenant, err := store.Tenant(ctx, tx)
	if err != nil {
		return err
	}
	definitions := make(map[string]model.CustomNumbering, len(def.CustomNumberings))
	for _, numbering := range def.CustomNumberings {
		definitions[numbering.ID] = numbering
	}
	for _, setting := range def.CustomNumberingSettings {
		if !pf14NumberingDue(setting.TimingType, event) || setting.TicketNumberFormat == nil || *setting.TicketNumberFormat == "" {
			continue
		}
		if event == numberingEventSubStatusAttached && setting.SubStatusID != nil {
			activeID, err := store.TicketSubStatusID(ctx, tx, ticket.ID)
			if err != nil {
				return err
			}
			if activeID == nil || *activeID != *setting.SubStatusID {
				continue
			}
		}
		definition, ok := definitions[setting.CustomNumberingID]
		if !ok {
			return fmt.Errorf("engine: custom numbering setting references missing numbering %q", setting.CustomNumberingID)
		}
		var assigned string
		err := tx.QueryRowContext(ctx, `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, ticket.ID, definition.ID).Scan(&assigned)
		if err == nil {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("engine: read ticket custom numbering: %w", err)
		}
		counterKey := "custom:" + definition.ID
		if definition.TicketNumberKeyID != nil && *definition.TicketNumberKeyID != "" {
			counterKey = "key:" + *definition.TicketNumberKeyID
		}
		initial := definition.Value
		if initial < 1 {
			initial = 1
		}
		var counter int64
		if _, err = tx.ExecContext(ctx, `INSERT INTO custom_numbering_counters(custom_numbering_id,counter) VALUES(?,?) ON CONFLICT(custom_numbering_id) DO NOTHING`, counterKey, initial-1); err != nil {
			return fmt.Errorf("engine: initialize custom numbering counter: %w", err)
		}
		if err = tx.QueryRowContext(ctx, `UPDATE custom_numbering_counters SET counter=counter+1 WHERE custom_numbering_id=? RETURNING counter`, counterKey).Scan(&counter); err != nil {
			return fmt.Errorf("engine: increment custom numbering counter: %w", err)
		}
		formatted := pf14FormatTicketNumber(*setting.TicketNumberFormat, now, counter, tenant.FiscalYearStartMonth)
		if _, err = tx.ExecContext(ctx, `INSERT INTO ticket_custom_numbering_values(ticket_id,custom_numbering_id,formatted_number) VALUES(?,?,?)`, ticket.ID, definition.ID, formatted); err != nil {
			return fmt.Errorf("engine: save ticket custom numbering: %w", err)
		}
	}
	return nil
}

// pf14FormatTicketNumber expands the documented format tokens in JST:
// [year], [month] and [day] (zero padded to 4, 2 and 2 digits), [fyear] (the
// fiscal year, which starts in April and is named after its starting calendar
// year) and [number] (the per-workflow counter, not padded).
func pf14FormatTicketNumber(format string, now time.Time, counter int64, fiscalYearStartMonth int) string {
	local := now.In(kftime.JST)
	if fiscalYearStartMonth < 1 || fiscalYearStartMonth > 12 {
		fiscalYearStartMonth = 4
	}
	fiscalYear := local.Year()
	if int(local.Month()) < fiscalYearStartMonth {
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
