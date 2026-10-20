package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"punchflow/internal/model"
)

// AdminListTokens returns a page of tokens owned by userID, or all service accounts when userID is empty.
func AdminListTokens(ctx context.Context, q Querier, userID string, offset, limit int) ([]model.APIToken, int, error) {
	where, args := "user_id=? AND kind='pat'", []any{userID}
	if userID == "" {
		where, args = "kind='sat'", nil
	}
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM api_tokens WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count admin tokens: %w", err)
	}
	args = append(args, limit, offset)
	rows, err := q.QueryContext(ctx, "SELECT id,kind,name,description,user_id,created_at,last_used_at FROM api_tokens WHERE "+where+" ORDER BY created_at,id LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list admin tokens: %w", err)
	}
	defer rows.Close()
	out := make([]model.APIToken, 0)
	for rows.Next() {
		var t model.APIToken
		var desc, owner sql.NullString
		var created int64
		var used sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Kind, &t.Name, &desc, &owner, &created, &used); err != nil {
			return nil, 0, fmt.Errorf("store: scan admin token: %w", err)
		}
		if desc.Valid {
			t.Description = &desc.String
		}
		if owner.Valid {
			t.UserID = &owner.String
		}
		t.CreatedAt = Time(created)
		t.LastUsedAt = TimePtr(used)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate admin tokens: %w", err)
	}
	return out, total, nil
}

// AdminDeleteToken deletes a token only when it belongs to userID, unless integration is true.
func AdminDeleteToken(ctx context.Context, q Querier, id, userID string, integration bool) error {
	query, args := "DELETE FROM api_tokens WHERE id=? AND kind='pat' AND user_id=?", []any{id, userID}
	if integration {
		query, args = "DELETE FROM api_tokens WHERE id=? AND kind='pat'", []any{id}
	}
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: delete admin token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count deleted admin token: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminDeleteServiceAccount deletes a SAT by id.
func AdminDeleteServiceAccount(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, "DELETE FROM api_tokens WHERE id=? AND kind='sat'", id)
	if err != nil {
		return fmt.Errorf("store: delete service account: %w", err)
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

// AdminUpdateTenant applies the mutable tenant settings.
func AdminUpdateTenant(ctx context.Context, q Querier, t model.Tenant) error {
	allow, err := json.Marshal(t.IPAllowlist)
	if err != nil {
		return err
	}
	domains, err := json.Marshal(t.AllowedEmailDomains)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, "UPDATE tenant SET public_id=?,name=?,plan=?,mode=?,ip_allowlist=?,allowed_email_domains=?,self_service_proxy=?,updated_at=? WHERE id=?", t.PublicID, t.Name, t.Plan, t.Mode, string(allow), string(domains), t.SelfServiceProxy, Millis(t.UpdatedAt), t.ID)
	if err != nil {
		return fmt.Errorf("store: update tenant: %w", err)
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
