package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"punchflow/internal/model"
)

const labelColumns = `id,name,description,color,created_at,updated_at`
const subStatusColumns = `id,code,name,notes,allow_withdrawal,allow_rejection,allow_denial,created_at,updated_at`

type classificationRowScanner interface{ Scan(dest ...any) error }

func scanLabel(r classificationRowScanner) (model.Label, error) {
	var x model.Label
	var description sql.NullString
	var created, updated int64
	if err := r.Scan(&x.ID, &x.Name, &description, &x.Color, &created, &updated); err != nil {
		return x, err
	}
	x.Description = pf15StringPtr(description)
	x.CreatedAt, x.UpdatedAt = Time(created), Time(updated)
	return x, nil
}

func scanSubStatus(r classificationRowScanner) (model.SubStatus, error) {
	var x model.SubStatus
	var notes sql.NullString
	var withdrawal, rejection, denial int
	var created, updated int64
	if err := r.Scan(&x.ID, &x.Code, &x.Name, &notes, &withdrawal, &rejection, &denial, &created, &updated); err != nil {
		return x, err
	}
	x.Notes = pf15StringPtr(notes)
	x.AllowWithdrawal, x.AllowRejection, x.AllowDenial = withdrawal != 0, rejection != 0, denial != 0
	x.CreatedAt, x.UpdatedAt = Time(created), Time(updated)
	return x, nil
}

// Labels returns every label ordered by name.
func Labels(ctx context.Context, q Querier) ([]model.Label, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+labelColumns+` FROM labels ORDER BY name,id`)
	if err != nil {
		return nil, fmt.Errorf("store: query labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.Label{}
	for rows.Next() {
		x, err := scanLabel(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan label: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LabelByID returns one label or ErrNotFound.
func LabelByID(ctx context.Context, q Querier, id string) (model.Label, error) {
	x, err := scanLabel(q.QueryRowContext(ctx, `SELECT `+labelColumns+` FROM labels WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, fmt.Errorf("store: load label: %w", err)
	}
	return x, nil
}

// SubStatuses returns every sub-status ordered by code.
func SubStatuses(ctx context.Context, q Querier) ([]model.SubStatus, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+subStatusColumns+` FROM sub_statuses ORDER BY code,id`)
	if err != nil {
		return nil, fmt.Errorf("store: query sub-statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.SubStatus{}
	for rows.Next() {
		x, err := scanSubStatus(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan sub-status: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SubStatusByID returns one sub-status or ErrNotFound.
func SubStatusByID(ctx context.Context, q Querier, id string) (model.SubStatus, error) {
	x, err := scanSubStatus(q.QueryRowContext(ctx, `SELECT `+subStatusColumns+` FROM sub_statuses WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, fmt.Errorf("store: load sub-status: %w", err)
	}
	return x, nil
}

// SubStatusByCode returns one sub-status or ErrNotFound.
func SubStatusByCode(ctx context.Context, q Querier, code string) (model.SubStatus, error) {
	x, err := scanSubStatus(q.QueryRowContext(ctx, `SELECT `+subStatusColumns+` FROM sub_statuses WHERE code=?`, code))
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, fmt.Errorf("store: load sub-status by code: %w", err)
	}
	return x, nil
}

// TicketSubStatusID returns the sub-status currently attached to a ticket, or nil.
func TicketSubStatusID(ctx context.Context, q Querier, ticketID string) (*string, error) {
	var id sql.NullString
	err := q.QueryRowContext(ctx, `SELECT sub_status_id FROM tickets WHERE id=?`, ticketID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: read ticket sub-status: %w", err)
	}
	return pf15StringPtr(id), nil
}

// SetTicketSubStatus attaches (or, with nil, clears) the ticket's sub-status.
func SetTicketSubStatus(ctx context.Context, q Querier, ticketID string, subStatusID *string) error {
	if _, err := q.ExecContext(ctx, `UPDATE tickets SET sub_status_id=? WHERE id=?`, subStatusID, ticketID); err != nil {
		return fmt.Errorf("store: set ticket sub-status: %w", err)
	}
	return nil
}

// TicketLabels returns the labels attached to a ticket ordered by name.
func TicketLabels(ctx context.Context, q Querier, ticketID string) ([]model.Label, error) {
	rows, err := q.QueryContext(ctx, `SELECT l.id,l.name,l.description,l.color,l.created_at,l.updated_at FROM labels l JOIN ticket_labels tl ON tl.label_id=l.id WHERE tl.ticket_id=? ORDER BY l.name,l.id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.Label{}
	for rows.Next() {
		x, err := scanLabel(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan ticket label: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// AddTicketLabel attaches a label; attaching an attached label is a no-op.
// It reports whether the association was newly created.
func AddTicketLabel(ctx context.Context, q Querier, ticketID, labelID string, now int64) (bool, error) {
	res, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO ticket_labels(ticket_id,label_id,created_at) VALUES(?,?,?)`, ticketID, labelID, now)
	if err != nil {
		return false, fmt.Errorf("store: attach ticket label: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RemoveTicketLabel detaches a label and reports whether one was attached.
func RemoveTicketLabel(ctx context.Context, q Querier, ticketID, labelID string) (bool, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM ticket_labels WHERE ticket_id=? AND label_id=?`, ticketID, labelID)
	if err != nil {
		return false, fmt.Errorf("store: detach ticket label: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// EngShiftStepOrders moves every step of the ticket whose order is greater
// than after up by one, opening a free order slot after "after".
func EngShiftStepOrders(ctx context.Context, q Querier, ticketID string, after int) error {
	if _, err := q.ExecContext(ctx, `UPDATE ticket_steps SET step_order=step_order+1 WHERE ticket_id=? AND step_order>?`, ticketID, after); err != nil {
		return fmt.Errorf("store: shift step orders: %w", err)
	}
	return nil
}

// EngCloseStepOrderGap moves steps above "removed" down by one.
func EngCloseStepOrderGap(ctx context.Context, q Querier, ticketID string, removed int) error {
	if _, err := q.ExecContext(ctx, `UPDATE ticket_steps SET step_order=step_order-1 WHERE ticket_id=? AND step_order>?`, ticketID, removed); err != nil {
		return fmt.Errorf("store: close step order gap: %w", err)
	}
	return nil
}

// EngSetStepDisplayOrder updates one step's display order.
func EngSetStepDisplayOrder(ctx context.Context, q Querier, stepID string, display int) error {
	if _, err := q.ExecContext(ctx, `UPDATE ticket_steps SET display_order=? WHERE id=?`, display, stepID); err != nil {
		return fmt.Errorf("store: set step display order: %w", err)
	}
	return nil
}

// EngDeleteStep removes one ticket step (assignees cascade).
func EngDeleteStep(ctx context.Context, q Querier, stepID string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM ticket_steps WHERE id=?`, stepID); err != nil {
		return fmt.Errorf("store: delete ticket step: %w", err)
	}
	return nil
}
