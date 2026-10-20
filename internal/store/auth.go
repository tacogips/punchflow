package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/model"
)

func pf05UserScan(row interface{ Scan(...any) error }) (model.User, error) {
	var u model.User
	var employee, last sql.NullString
	var deactivated sql.NullInt64
	var custom string
	var created, updated int64
	err := row.Scan(&u.ID, &u.Code, &u.Email, &u.FirstName, &u.LastName, &employee, &u.Status, &u.Locale, &u.UserType, &custom, &last, &deactivated, &created, &updated)
	if err != nil {
		return model.User{}, err
	}
	if employee.Valid {
		u.EmployeeID = &employee.String
	}
	if last.Valid {
		u.LastUsedOn = &last.String
	}
	u.DeactivatedAt = TimePtr(deactivated)
	u.CreatedAt, u.UpdatedAt = Time(created), Time(updated)
	if err := json.Unmarshal([]byte(custom), &u.CustomFields); err != nil {
		return model.User{}, fmt.Errorf("store: decode user custom fields: %w", err)
	}
	if u.CustomFields == nil {
		u.CustomFields = map[string]any{}
	}
	return u, nil
}

const pf05UserColumns = "id,code,email,first_name,last_name,employee_id,status,locale,user_type,custom_fields,last_used_on,deactivated_at,created_at,updated_at"

// TokenByHash retrieves an API token by its stored digest.
func TokenByHash(ctx context.Context, q Querier, hash string) (model.APIToken, error) {
	var t model.APIToken
	var desc, user, fixture sql.NullString
	var last sql.NullInt64
	var created int64
	err := q.QueryRowContext(ctx, "SELECT id,kind,name,description,token_hash,user_id,fixture_plaintext,created_at,last_used_at FROM api_tokens WHERE token_hash=?", hash).Scan(&t.ID, &t.Kind, &t.Name, &desc, &t.TokenHash, &user, &fixture, &created, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return model.APIToken{}, ErrNotFound
	}
	if err != nil {
		return model.APIToken{}, fmt.Errorf("store: read token: %w", err)
	}
	if desc.Valid {
		t.Description = &desc.String
	}
	if user.Valid {
		t.UserID = &user.String
	}
	if fixture.Valid {
		t.FixturePlaintext = &fixture.String
	}
	t.CreatedAt = Time(created)
	t.LastUsedAt = TimePtr(last)
	return t, nil
}

// TouchToken records token use time.
func TouchToken(ctx context.Context, q Querier, id string, at time.Time) error {
	if _, err := q.ExecContext(ctx, "UPDATE api_tokens SET last_used_at=? WHERE id=?", Millis(at), id); err != nil {
		return fmt.Errorf("store: touch token: %w", err)
	}
	return nil
}

// InsertToken stores a token digest and metadata.
func InsertToken(ctx context.Context, q Querier, t model.APIToken) error {
	_, err := q.ExecContext(ctx, "INSERT INTO api_tokens(id,kind,name,description,token_hash,user_id,fixture_plaintext,created_at,last_used_at) VALUES(?,?,?,?,?,?,?,?,?)", t.ID, t.Kind, t.Name, t.Description, t.TokenHash, t.UserID, t.FixturePlaintext, Millis(t.CreatedAt), NullMillis(t.LastUsedAt))
	if err != nil {
		return fmt.Errorf("store: insert token: %w", err)
	}
	return nil
}

// UserByID retrieves a user by ID.
func UserByID(ctx context.Context, q Querier, id string) (model.User, error) {
	u, err := pf05UserScan(q.QueryRowContext(ctx, "SELECT "+pf05UserColumns+" FROM users WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("store: read user: %w", err)
	}
	return u, nil
}

// UserByEmail retrieves a user by case-insensitive email.
func UserByEmail(ctx context.Context, q Querier, email string) (model.User, error) {
	u, err := pf05UserScan(q.QueryRowContext(ctx, "SELECT "+pf05UserColumns+" FROM users WHERE email=? COLLATE NOCASE", email))
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("store: read user by email: %w", err)
	}
	return u, nil
}

// UserPermissions returns the union of permission names from assigned roles.
func UserPermissions(ctx context.Context, q Querier, userID string) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT r.permission_list FROM roles r JOIN role_memberships rm ON rm.role_id=r.id WHERE rm.user_id=?", userID)
	if err != nil {
		return nil, fmt.Errorf("store: query user permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("store: scan permissions: %w", err)
		}
		var entries []struct {
			Permission string `json:"permission"`
		}
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, fmt.Errorf("store: decode permissions: %w", err)
		}
		for _, entry := range entries {
			if entry.Permission != "" {
				out[entry.Permission] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read permissions: %w", err)
	}
	return out, nil
}

// InvitationByUser retrieves the user's invitation, if one exists.
func InvitationByUser(ctx context.Context, q Querier, userID string) (model.UserInvitation, error) {
	var v model.UserInvitation
	var expires, created, updated int64
	err := q.QueryRowContext(ctx, "SELECT id,user_id,user_email,user_locale,expires_at,created_at,updated_at FROM user_invitations WHERE user_id=?", userID).Scan(&v.ID, &v.UserID, &v.UserEmail, &v.UserLocale, &expires, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.UserInvitation{}, ErrNotFound
	}
	if err != nil {
		return model.UserInvitation{}, fmt.Errorf("store: read invitation: %w", err)
	}
	v.ExpiresAt = Time(expires)
	v.CreatedAt = Time(created)
	v.UpdatedAt = Time(updated)
	return v, nil
}

// ActivateUser changes an invited user to active.
func ActivateUser(ctx context.Context, q Querier, userID string, at time.Time) error {
	res, err := q.ExecContext(ctx, "UPDATE users SET status=?,updated_at=? WHERE id=? AND status=?", model.UserStatusActive, Millis(at), userID, model.UserStatusInvited)
	if err != nil {
		return fmt.Errorf("store: activate user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: inspect activation: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsedOn updates a user's JST last-used date only when it changes.
func TouchLastUsedOn(ctx context.Context, q Querier, userID, date string) error {
	if _, err := q.ExecContext(ctx, "UPDATE users SET last_used_on=? WHERE id=? AND (last_used_on IS NULL OR last_used_on<>?)", date, userID, date); err != nil {
		return fmt.Errorf("store: touch last used date: %w", err)
	}
	return nil
}

// InsertSession persists a session using only the hash of its cookie value.
func InsertSession(ctx context.Context, q Querier, s model.Session) error {
	_, err := q.ExecContext(ctx, "INSERT INTO sessions(id_hash,user_id,csrf_token,created_at,last_seen_at) VALUES(?,?,?,?,?)", s.IDHash, s.UserID, s.CSRFToken, Millis(s.CreatedAt), Millis(s.LastSeenAt))
	if err != nil {
		return fmt.Errorf("store: insert session: %w", err)
	}
	return nil
}

// SessionByHash retrieves a session by cookie digest.
func SessionByHash(ctx context.Context, q Querier, hash string) (model.Session, error) {
	var s model.Session
	var created, last int64
	err := q.QueryRowContext(ctx, "SELECT id_hash,user_id,csrf_token,created_at,last_seen_at FROM sessions WHERE id_hash=?", hash).Scan(&s.IDHash, &s.UserID, &s.CSRFToken, &created, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Session{}, ErrNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("store: read session: %w", err)
	}
	s.CreatedAt = Time(created)
	s.LastSeenAt = Time(last)
	return s, nil
}

// TouchSession records session activity.
func TouchSession(ctx context.Context, q Querier, hash string, at time.Time) error {
	if _, err := q.ExecContext(ctx, "UPDATE sessions SET last_seen_at=? WHERE id_hash=?", Millis(at), hash); err != nil {
		return fmt.Errorf("store: touch session: %w", err)
	}
	return nil
}

// DeleteSession deletes a session by cookie digest.
func DeleteSession(ctx context.Context, q Querier, hash string) error {
	if _, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE id_hash=?", hash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// Tenant retrieves the single tenant record.
func Tenant(ctx context.Context, q Querier) (model.Tenant, error) {
	var t model.Tenant
	var allow, domains string
	var proxy int
	var created, updated int64
	err := q.QueryRowContext(ctx, "SELECT id,public_id,name,plan,mode,ip_allowlist,allowed_email_domains,self_service_proxy,created_at,updated_at FROM tenant LIMIT 1").Scan(&t.ID, &t.PublicID, &t.Name, &t.Plan, &t.Mode, &allow, &domains, &proxy, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Tenant{}, ErrNotFound
	}
	if err != nil {
		return model.Tenant{}, fmt.Errorf("store: read tenant: %w", err)
	}
	if err = json.Unmarshal([]byte(allow), &t.IPAllowlist); err != nil {
		return model.Tenant{}, fmt.Errorf("store: decode IP allowlist: %w", err)
	}
	if err = json.Unmarshal([]byte(domains), &t.AllowedEmailDomains); err != nil {
		return model.Tenant{}, fmt.Errorf("store: decode email domains: %w", err)
	}
	t.SelfServiceProxy = proxy != 0
	t.CreatedAt = Time(created)
	t.UpdatedAt = Time(updated)
	return t, nil
}

// InsertOIDCState stores a single-use OIDC login state.
func InsertOIDCState(ctx context.Context, q Querier, s model.OIDCState) error {
	_, err := q.ExecContext(ctx, "INSERT INTO oidc_states(state,nonce,code_verifier,return_to,expires_at) VALUES(?,?,?,?,?)", s.State, s.Nonce, s.CodeVerifier, s.ReturnTo, Millis(s.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: insert OIDC state: %w", err)
	}
	return nil
}

// TakeOIDCState atomically consumes and returns an OIDC state.
func TakeOIDCState(ctx context.Context, q Querier, state string) (model.OIDCState, error) {
	var s model.OIDCState
	var exp int64
	err := q.QueryRowContext(ctx, "DELETE FROM oidc_states WHERE state=? RETURNING state,nonce,code_verifier,return_to,expires_at", state).Scan(&s.State, &s.Nonce, &s.CodeVerifier, &s.ReturnTo, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return model.OIDCState{}, ErrNotFound
	}
	if err != nil {
		return model.OIDCState{}, fmt.Errorf("store: consume OIDC state: %w", err)
	}
	s.ExpiresAt = Time(exp)
	return s, nil
}
