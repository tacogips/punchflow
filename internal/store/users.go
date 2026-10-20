package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"punchflow/internal/model"
)

// UserListFilter contains the supported listUsers filters.
type UserListFilter struct {
	Statuses []string
}

// UserPatch contains fields that can be changed by updateUser. EmployeeID is a
// pointer to a pointer so nil means omitted and a pointer to nil means clear.
type UserPatch struct {
	Code         *string
	Email        *string
	FirstName    *string
	LastName     *string
	EmployeeID   **string
	CustomFields map[string]any
}

// UserList returns matching user IDs in stable order and the unpaginated count.
func UserList(ctx context.Context, q Querier, filter UserListFilter, offset, limit int, sortBy string, desc bool) ([]string, int, error) {
	where := ""
	args := []any{}
	if len(filter.Statuses) > 0 {
		marks := make([]string, len(filter.Statuses))
		for i, status := range filter.Statuses {
			marks[i] = "?"
			args = append(args, status)
		}
		where = " WHERE status IN (" + strings.Join(marks, ",") + ")"
	}
	order := map[string]string{"email": "email", "code": "code", "lastUsedOn": "last_used_on"}[sortBy]
	if order == "" {
		order = "email"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM users"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count users: %w", err)
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := q.QueryContext(ctx, "SELECT id FROM users"+where+" ORDER BY "+order+",id LIMIT ? OFFSET ?", listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list users: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate users: %w", err)
	}
	return ids, total, nil
}

// UserCreate inserts a user row. Callers should create invitations and
// memberships in the same transaction when the endpoint requires them.
func UserCreate(ctx context.Context, q Querier, user model.User) error {
	custom, err := json.Marshal(user.CustomFields)
	if err != nil {
		return fmt.Errorf("store: encode user custom fields: %w", err)
	}
	if user.CustomFields == nil {
		custom = []byte("{}")
	}
	_, err = q.ExecContext(ctx, `INSERT INTO users(id,code,email,first_name,last_name,employee_id,status,locale,user_type,custom_fields,last_used_on,deactivated_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		user.ID, user.Code, user.Email, user.FirstName, user.LastName, user.EmployeeID, user.Status, user.Locale, user.UserType, string(custom), user.LastUsedOn, NullMillis(user.DeactivatedAt), Millis(user.CreatedAt), Millis(user.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create user: %w", err)
	}
	return nil
}

// UserUpdate applies only supplied fields to an existing user.
func UserUpdate(ctx context.Context, q Querier, id string, patch UserPatch, updatedAt int64) error {
	sets := []string{}
	args := []any{}
	if patch.Code != nil {
		sets = append(sets, "code=?")
		args = append(args, *patch.Code)
	}
	if patch.Email != nil {
		sets = append(sets, "email=?")
		args = append(args, *patch.Email)
	}
	if patch.FirstName != nil {
		sets = append(sets, "first_name=?")
		args = append(args, *patch.FirstName)
	}
	if patch.LastName != nil {
		sets = append(sets, "last_name=?")
		args = append(args, *patch.LastName)
	}
	if patch.EmployeeID != nil {
		sets = append(sets, "employee_id=?")
		if *patch.EmployeeID == nil {
			args = append(args, nil)
		} else {
			args = append(args, **patch.EmployeeID)
		}
	}
	if patch.CustomFields != nil {
		custom, err := json.Marshal(patch.CustomFields)
		if err != nil {
			return fmt.Errorf("store: encode user custom fields: %w", err)
		}
		sets = append(sets, "custom_fields=?")
		args = append(args, string(custom))
	}
	sets = append(sets, "updated_at=?")
	args = append(args, updatedAt, id)
	result, err := q.ExecContext(ctx, "UPDATE users SET "+strings.Join(sets, ",")+" WHERE id=?", args...)
	if err != nil {
		return fmt.Errorf("store: update user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count updated users: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserSetStatus changes a user's status and optional deactivation timestamp.
func UserSetStatus(ctx context.Context, q Querier, id, status string, deactivatedAt *time.Time, updatedAt int64) error {
	result, err := q.ExecContext(ctx, `UPDATE users SET status=?,deactivated_at=?,updated_at=? WHERE id=?`, status, NullMillis(deactivatedAt), updatedAt, id)
	if err != nil {
		return fmt.Errorf("store: update user status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count updated users: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserEmailAllowed checks an address against the tenant's optional domain list.
// An empty list permits every domain.
func UserEmailAllowed(ctx context.Context, q Querier, email string) (bool, error) {
	var raw string
	if err := q.QueryRowContext(ctx, `SELECT allowed_email_domains FROM tenant LIMIT 1`).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, fmt.Errorf("store: read allowed email domains: %w", err)
	}
	var domains []string
	if err := json.Unmarshal([]byte(raw), &domains); err != nil {
		return false, fmt.Errorf("store: decode allowed email domains: %w", err)
	}
	if len(domains) == 0 {
		return true, nil
	}
	_, domain, ok := strings.Cut(email, "@")
	if !ok || domain == "" {
		return false, nil
	}
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	for _, allowed := range domains {
		allowed = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(allowed)), "@")
		allowed = strings.TrimSuffix(allowed, ".")
		if domain == allowed {
			return true, nil
		}
	}
	return false, nil
}

// UserSetInvitation inserts or replaces the invitation associated with a user.
func UserSetInvitation(ctx context.Context, q Querier, invitation model.UserInvitation) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET id=excluded.id,user_email=excluded.user_email,user_locale=excluded.user_locale,expires_at=excluded.expires_at,created_at=excluded.created_at,updated_at=excluded.updated_at`,
		invitation.ID, invitation.UserID, invitation.UserEmail, invitation.UserLocale, Millis(invitation.ExpiresAt), Millis(invitation.CreatedAt), Millis(invitation.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: set user invitation: %w", err)
	}
	return nil
}

// UserTeamIDs returns a user's memberships in the current organization chart.
func UserTeamIDs(ctx context.Context, q Querier, userID string, offset, limit int) ([]string, int, error) {
	var chartID string
	err := q.QueryRowContext(ctx, `SELECT id FROM organization_charts WHERE current=1 ORDER BY created_at DESC LIMIT 1`).Scan(&chartID)
	if err == sql.ErrNoRows {
		return []string{}, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("store: find current organization chart: %w", err)
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM team_memberships m JOIN teams t ON t.id=m.team_id WHERE m.user_id=? AND t.organization_chart_id=?`, userID, chartID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count user teams: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT t.id FROM team_memberships m JOIN teams t ON t.id=m.team_id WHERE m.user_id=? AND t.organization_chart_id=? ORDER BY t.code,t.id LIMIT ? OFFSET ?`, userID, chartID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list user teams: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan user team: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate user teams: %w", err)
	}
	return ids, total, nil
}

// UserRoleIDs returns a user's role IDs in stable order.
func UserRoleIDs(ctx context.Context, q Querier, userID string, offset, limit int) ([]string, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM role_memberships WHERE user_id=?`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count user roles: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT role_id FROM role_memberships WHERE user_id=? ORDER BY role_id LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list user roles: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan user role: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate user roles: %w", err)
	}
	return ids, total, nil
}

// UserAddTeamMembership inserts a membership as part of user invitation.
func UserAddTeamMembership(ctx context.Context, q Querier, membership model.TeamMembership) error {
	grades, err := json.Marshal(membership.GradeIDs)
	if err != nil {
		return fmt.Errorf("store: encode membership grades: %w", err)
	}
	if membership.GradeIDs == nil {
		grades = []byte("[]")
	}
	_, err = q.ExecContext(ctx, `INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, membership.ID, membership.TeamID, membership.UserID, membership.Leader, string(grades), Millis(membership.CreatedAt), Millis(membership.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create user team membership: %w", err)
	}
	return nil
}
