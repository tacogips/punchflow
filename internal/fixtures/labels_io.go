package fixtures

import (
	"context"
	"database/sql"

	"punchflow/internal/store"
)

// insertLabelsAndSubStatuses persists the label and sub-status fixtures.
func insertLabelsAndSubStatuses(insert func(string, string, ...any) error, s *Set, ms int64) error {
	for _, l := range s.Labels {
		if err := insert("labels", "INSERT INTO labels(id,name,description,color,created_at,updated_at) VALUES(?,?,?,?,?,?)", l.ID, l.Name, l.Description, l.Color, ms, ms); err != nil {
			return err
		}
	}
	for _, x := range s.SubStatuses {
		if err := insert("sub_statuses", "INSERT INTO sub_statuses(id,code,name,notes,allow_withdrawal,allow_rejection,allow_denial,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", x.ID, x.Code, x.Name, x.Notes, x.AllowWithdrawal, x.AllowRejection, x.AllowDenial, ms, ms); err != nil {
			return err
		}
	}
	return nil
}

// exportLabelsAndSubStatuses reads both definition tables into the set.
func exportLabelsAndSubStatuses(ctx context.Context, q store.Querier, s *Set) error {
	if err := pf06Query(ctx, q, "SELECT id,name,description,color FROM labels ORDER BY name,id", func(r *sql.Rows) error {
		var x LabelFixture
		var description sql.NullString
		if e := r.Scan(&x.ID, &x.Name, &description, &x.Color); e != nil {
			return e
		}
		if description.Valid {
			x.Description = &description.String
		}
		s.Labels = append(s.Labels, x)
		return nil
	}); err != nil {
		return err
	}
	return pf06Query(ctx, q, "SELECT id,code,name,notes,allow_withdrawal,allow_rejection,allow_denial FROM sub_statuses ORDER BY code,id", func(r *sql.Rows) error {
		var x SubStatusFixture
		var notes sql.NullString
		if e := r.Scan(&x.ID, &x.Code, &x.Name, &notes, &x.AllowWithdrawal, &x.AllowRejection, &x.AllowDenial); e != nil {
			return e
		}
		if notes.Valid {
			x.Notes = &notes.String
		}
		s.SubStatuses = append(s.SubStatuses, x)
		return nil
	})
}
