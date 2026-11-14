package store

import (
	"context"
	"fmt"
	"time"
)

// ApplyEffectiveProxySharing snapshots every currently effective proxy
// approver onto processing tickets. It is safe to run on authenticated reads:
// viewer rows are idempotent and automatic watches only fill an unset choice.
func ApplyEffectiveProxySharing(ctx context.Context, db *DB, today string, now time.Time) error {
	return db.WithTx(ctx, func(tx *Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT t.id,pa.proxy_user_id
			FROM tickets t
			JOIN ticket_assignees ta ON ta.ticket_id=t.id
			JOIN proxy_approvers pa ON pa.user_id=ta.user_id
			WHERE t.status='in_progress'
			  AND (pa.starts_on IS NULL OR pa.starts_on<=?)
			  AND (pa.ends_on IS NULL OR pa.ends_on>=?)
			  AND (pa.workflow_ids='[]' OR EXISTS (
			      SELECT 1 FROM json_each(pa.workflow_ids) AS workflow
			      WHERE workflow.value=t.workflow_id))
			ORDER BY t.id,pa.proxy_user_id`, today, today)
		if err != nil {
			return fmt.Errorf("store: list effective proxy shares: %w", err)
		}
		type relation struct{ ticketID, proxyID string }
		var relations []relation
		for rows.Next() {
			var r relation
			if err := rows.Scan(&r.ticketID, &r.proxyID); err != nil {
				_ = rows.Close()
				return fmt.Errorf("store: scan effective proxy share: %w", err)
			}
			relations = append(relations, r)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: iterate effective proxy shares: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("store: close effective proxy shares: %w", err)
		}
		for _, r := range relations {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ticket_viewers WHERE ticket_id=? AND user_id=?`, r.ticketID, r.proxyID).Scan(&exists); err != nil {
				return fmt.Errorf("store: check proxy ticket viewer: %w", err)
			}
			if exists == 0 {
				if _, err := tx.ExecContext(ctx, `INSERT INTO ticket_viewers(id,ticket_id,user_id) VALUES(?,?,?)`, NewID(), r.ticketID, r.proxyID); err != nil {
					return fmt.Errorf("store: add proxy ticket viewer: %w", err)
				}
			}
			if err := SetTicketWatchStateIfUnset(ctx, tx, r.ticketID, r.proxyID, WatchStateWatching, now); err != nil {
				return err
			}
		}
		return nil
	})
}
