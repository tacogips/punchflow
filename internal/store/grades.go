package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"punchflow/internal/model"
)

// GradePatch contains fields that can be changed by updateGrade.
type GradePatch struct {
	Code  *string
	Name  *string
	Level *int
}

// GradeList returns grade IDs in the requested order and the total count.
func GradeList(ctx context.Context, q Querier, offset, limit int, sortBy string, desc bool) ([]string, int, error) {
	order := map[string]string{"level": "level", "code": "code"}[sortBy]
	if order == "" {
		order = "level"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM grades`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count grades: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM grades ORDER BY `+order+`,id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list grades: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("store: scan grade id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate grades: %w", err)
	}
	return ids, total, nil
}

// GradeCreate inserts a grade row.
func GradeCreate(ctx context.Context, q Querier, grade model.Grade) error {
	_, err := q.ExecContext(ctx, `INSERT INTO grades(id,code,name,level,is_default,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, grade.ID, grade.Code, grade.Name, grade.Level, grade.IsDefault, Millis(grade.CreatedAt), Millis(grade.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create grade: %w", err)
	}
	return nil
}

// GradeUpdate applies only the supplied fields to an existing grade.
func GradeUpdate(ctx context.Context, q Querier, id string, patch GradePatch, updatedAt int64) error {
	sets := []string{}
	args := []any{}
	if patch.Code != nil {
		sets = append(sets, "code=?")
		args = append(args, *patch.Code)
	}
	if patch.Name != nil {
		sets = append(sets, "name=?")
		args = append(args, *patch.Name)
	}
	if patch.Level != nil {
		sets = append(sets, "level=?")
		args = append(args, *patch.Level)
	}
	sets = append(sets, "updated_at=?")
	args = append(args, updatedAt, id)
	result, err := q.ExecContext(ctx, `UPDATE grades SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	if err != nil {
		return fmt.Errorf("store: update grade: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count updated grades: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GradeDelete deletes a grade by ID.
func GradeDelete(ctx context.Context, q Querier, id string) error {
	result, err := q.ExecContext(ctx, `DELETE FROM grades WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("store: delete grade: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count deleted grades: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GradeInUse reports whether any team membership assigns the grade.
func GradeInUse(ctx context.Context, q Querier, id string) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT grade_ids FROM team_memberships`)
	if err != nil {
		return false, fmt.Errorf("store: list membership grades: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, fmt.Errorf("store: scan membership grades: %w", err)
		}
		var gradeIDs []string
		if err := json.Unmarshal([]byte(raw), &gradeIDs); err != nil {
			return false, fmt.Errorf("store: decode membership grades: %w", err)
		}
		for _, gradeID := range gradeIDs {
			if gradeID == id {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("store: iterate membership grades: %w", err)
	}
	return false, nil
}

// GradeSetDefault makes id the default grade and clears any previous default.
// It should be called with a transaction Querier for atomic endpoint behavior.
func GradeSetDefault(ctx context.Context, q Querier, id string, updatedAt int64) error {
	var exists string
	if err := q.QueryRowContext(ctx, `SELECT id FROM grades WHERE id=?`, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("store: find default grade: %w", err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE grades SET is_default=0,updated_at=? WHERE is_default<>0`, updatedAt); err != nil {
		return fmt.Errorf("store: clear default grade: %w", err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE grades SET is_default=1,updated_at=? WHERE id=?`, updatedAt, id); err != nil {
		return fmt.Errorf("store: set default grade: %w", err)
	}
	return nil
}

// DefaultGradeID returns the configured default grade.
func DefaultGradeID(ctx context.Context, q Querier) (string, error) {
	var id string
	if err := q.QueryRowContext(ctx, `SELECT id FROM grades WHERE is_default=1 LIMIT 1`).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: read default grade: %w", err)
	}
	return id, nil
}
