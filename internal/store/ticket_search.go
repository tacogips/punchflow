package store

import (
	"context"
	"fmt"
	"strings"
)

// likeEscaper escapes the LIKE wildcards so a keyword is matched literally.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// TicketKeywordCandidates returns the IDs of tickets in which every term occurs
// (case-insensitively for ASCII, as SQLite LIKE) in at least one searchable
// source: the ticket number, issued custom numbers, the title, a form input
// value, a detail row cell or a live comment body. It does not apply any
// visibility rule; callers must intersect the result with the tickets the
// requester may read, and re-check restricted form sections (see package
// ticketsearch).
func TicketKeywordCandidates(ctx context.Context, q Querier, terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	clauses := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*6)
	for _, term := range terms {
		pattern := "%" + likeEscaper.Replace(term) + "%"
		clauses = append(clauses, `(t.ticket_number LIKE ? ESCAPE '\' OR t.title LIKE ? ESCAPE '\'`+
			` OR EXISTS (SELECT 1 FROM ticket_custom_numbering_values n WHERE n.ticket_id=t.id AND n.formatted_number LIKE ? ESCAPE '\')`+
			` OR EXISTS (SELECT 1 FROM ticket_inputs i WHERE i.ticket_id=t.id AND CAST(CASE WHEN json_valid(i.value) THEN json_extract(i.value,'$') ELSE i.value END AS TEXT) LIKE ? ESCAPE '\')`+
			` OR EXISTS (SELECT 1 FROM ticket_slip_items s, json_each(s.inputs) cell WHERE s.ticket_id=t.id AND CAST(cell.value AS TEXT) LIKE ? ESCAPE '\')`+
			` OR EXISTS (SELECT 1 FROM comments c WHERE c.ticket_id=t.id AND c.deleted_at IS NULL AND c.body LIKE ? ESCAPE '\'))`)
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	rows, err := q.QueryContext(ctx, `SELECT t.id FROM tickets t WHERE `+strings.Join(clauses, " AND ")+` ORDER BY t.created_at DESC, t.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: keyword ticket candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan keyword candidate: %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate keyword candidates: %w", err)
	}
	return ids, nil
}

// TicketCustomNumberingValues returns the issued custom numbers for a ticket.
func TicketCustomNumberingValues(ctx context.Context, q Querier, ticketID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? ORDER BY custom_numbering_id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: list ticket custom numbering values: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("store: scan ticket custom numbering value: %w", err)
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ticket custom numbering values: %w", err)
	}
	return values, nil
}

// TicketLiveCommentBodies returns the bodies of the ticket's non-deleted
// comments.
func TicketLiveCommentBodies(ctx context.Context, q Querier, ticketID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT body FROM comments WHERE ticket_id=? AND deleted_at IS NULL ORDER BY created_at, id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: list comment bodies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var body string
		if err = rows.Scan(&body); err != nil {
			return nil, fmt.Errorf("store: scan comment body: %w", err)
		}
		out = append(out, body)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate comment bodies: %w", err)
	}
	return out, nil
}
