package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"punchflow/internal/model"
)

func ProxyListApplicants(ctx context.Context, q Querier, offset, limit int) ([]model.ProxyApplicant, int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM proxy_applicants ORDER BY created_at,id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list proxy applicants: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]model.ProxyApplicant, 0, len(ids))
	for _, id := range ids {
		v, e := GetProxyApplicant(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	total, err := pf08Count(ctx, q, "SELECT count(*) FROM proxy_applicants")
	return out, total, err
}
func ProxyListApprovers(ctx context.Context, q Querier, offset, limit int) ([]model.ProxyApprover, int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM proxy_approvers ORDER BY created_at,id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list proxy approvers: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]model.ProxyApprover, 0, len(ids))
	for _, id := range ids {
		v, e := GetProxyApprover(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	total, err := pf08Count(ctx, q, "SELECT count(*) FROM proxy_approvers")
	return out, total, err
}
func ProxyInsertApplicant(ctx context.Context, q Querier, v model.ProxyApplicant) error {
	return pf19ProxyInsert(ctx, q, "proxy_applicants", v.ID, v.UserID, v.ProxyUserID, v.StartsOn, v.EndsOn, v.WorkflowIDs, v.CreatedAt, v.UpdatedAt)
}
func ProxyInsertApprover(ctx context.Context, q Querier, v model.ProxyApprover) error {
	return pf19ProxyInsert(ctx, q, "proxy_approvers", v.ID, v.UserID, v.ProxyUserID, v.StartsOn, v.EndsOn, v.WorkflowIDs, v.CreatedAt, v.UpdatedAt)
}
func pf19ProxyInsert(ctx context.Context, q Querier, table, id, user, proxy string, starts, ends *string, workflows []string, created, updated interface{ UnixMilli() int64 }) error {
	encoded, err := json.Marshal(workflows)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, "INSERT INTO "+table+"(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", id, user, proxy, starts, ends, string(encoded), created.UnixMilli(), updated.UnixMilli())
	if err != nil {
		return fmt.Errorf("store: insert %s: %w", table, err)
	}
	return nil
}
func ProxyUpdateApplicant(ctx context.Context, q Querier, id string, v model.ProxyApplicant) error {
	return pf19ProxyUpdate(ctx, q, "proxy_applicants", id, v.UserID, v.ProxyUserID, v.StartsOn, v.EndsOn, v.WorkflowIDs, v.UpdatedAt.UnixMilli())
}
func ProxyUpdateApprover(ctx context.Context, q Querier, id string, v model.ProxyApprover) error {
	return pf19ProxyUpdate(ctx, q, "proxy_approvers", id, v.UserID, v.ProxyUserID, v.StartsOn, v.EndsOn, v.WorkflowIDs, v.UpdatedAt.UnixMilli())
}
func pf19ProxyUpdate(ctx context.Context, q Querier, table, id, user, proxy string, starts, ends *string, workflows []string, updated int64) error {
	encoded, err := json.Marshal(workflows)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, "UPDATE "+table+" SET user_id=?,proxy_user_id=?,starts_on=?,ends_on=?,workflow_ids=?,updated_at=? WHERE id=?", user, proxy, starts, ends, string(encoded), updated, id)
	if err != nil {
		return fmt.Errorf("store: update %s: %w", table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func ProxyDeleteApplicant(ctx context.Context, q Querier, id string) error {
	return pf19ProxyDelete(ctx, q, "proxy_applicants", id)
}
func ProxyDeleteApprover(ctx context.Context, q Querier, id string) error {
	return pf19ProxyDelete(ctx, q, "proxy_approvers", id)
}
func pf19ProxyDelete(ctx context.Context, q Querier, table, id string) error {
	res, err := q.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("store: delete %s: %w", table, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

var _ = sql.ErrNoRows
