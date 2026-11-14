package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"punchflow/internal/model"
)

func GetUser(ctx context.Context, q Querier, id string) (model.User, error) {
	var u model.User
	var custom string
	var last sql.NullString
	var deactivated sql.NullInt64
	var created, updated int64
	err := q.QueryRowContext(ctx, `SELECT id,code,email,first_name,last_name,employee_id,status,locale,user_type,custom_fields,last_used_on,deactivated_at,created_at,updated_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Code, &u.Email, &u.FirstName, &u.LastName, &u.EmployeeID, &u.Status, &u.Locale, &u.UserType, &custom, &last, &deactivated, &created, &updated)
	if err != nil {
		return u, pf08NotFound(err)
	}
	if err := json.Unmarshal([]byte(custom), &u.CustomFields); err != nil {
		return u, fmt.Errorf("store: decode user custom fields: %w", err)
	}
	if last.Valid {
		u.LastUsedOn = &last.String
	}
	if deactivated.Valid {
		t := Time(deactivated.Int64)
		u.DeactivatedAt = &t
	}
	u.CreatedAt = Time(created)
	u.UpdatedAt = Time(updated)
	return u, nil
}

func GetUsers(ctx context.Context, q Querier, ids []string) ([]model.User, error) {
	out := make([]model.User, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		u, err := GetUser(ctx, q, id)
		if err == nil {
			out = append(out, u)
		} else if err != ErrNotFound {
			return nil, err
		}
	}
	return out, nil
}

func GetUserInvitation(ctx context.Context, q Querier, userID string) (model.UserInvitation, error) {
	var x model.UserInvitation
	var exp, cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,user_id,user_email,user_locale,expires_at,created_at,updated_at FROM user_invitations WHERE user_id=?`, userID).Scan(&x.ID, &x.UserID, &x.UserEmail, &x.UserLocale, &exp, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.ExpiresAt = Time(exp)
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}

func UserIDByCode(ctx context.Context, q Querier, code string) (string, error) {
	return pf08IDBy(ctx, q, "users", code)
}
func GetGrade(ctx context.Context, q Querier, id string) (model.Grade, error) {
	var x model.Grade
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,name,level,is_default,created_at,updated_at FROM grades WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Name, &x.Level, &x.IsDefault, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetGrades(ctx context.Context, q Querier, ids []string) ([]model.Grade, error) {
	out := make([]model.Grade, 0, len(ids))
	for _, id := range ids {
		x, e := GetGrade(ctx, q, id)
		if e == nil {
			out = append(out, x)
		} else if e != ErrNotFound {
			return nil, e
		}
	}
	return out, nil
}
func GradeIDByCode(ctx context.Context, q Querier, code string) (string, error) {
	return pf08IDBy(ctx, q, "grades", code)
}
func pf08NotFound(err error) error {
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return fmt.Errorf("store: read row: %w", err)
}
func pf08IDBy(ctx context.Context, q Querier, table, code string) (string, error) {
	var id string
	if !strings.Contains(" users grades ", " "+table+" ") {
		return "", fmt.Errorf("store: invalid lookup table")
	}
	err := q.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE code=?", code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}

type pf08SQLNullString struct {
	s     string
	valid bool
}

func (n *pf08SQLNullString) Scan(src any) error {
	var v sql.NullString
	if err := v.Scan(src); err != nil {
		return err
	}
	n.s = v.String
	n.valid = v.Valid
	return nil
}
func (n pf08SQLNullString) ptr() *string {
	if !n.valid {
		return nil
	}
	v := n.s
	return &v
}
