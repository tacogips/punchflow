package store

import (
	"context"
	"encoding/json"
	"fmt"

	"punchflow/internal/model"
)

// UserCustomFieldList returns every user custom field definition in display order.
func UserCustomFieldList(ctx context.Context, q Querier) ([]model.UserCustomField, error) {
	rows, err := q.QueryContext(ctx, "SELECT id,code,name,field_type,options,visible_to_everyone,field_order,created_at,updated_at FROM user_custom_fields ORDER BY field_order,code")
	if err != nil {
		return nil, fmt.Errorf("store: list user custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []model.UserCustomField
	for rows.Next() {
		var f model.UserCustomField
		var options string
		var visible, created, updated int64
		if err := rows.Scan(&f.ID, &f.Code, &f.Name, &f.FieldType, &options, &visible, &f.FieldOrder, &created, &updated); err != nil {
			return nil, fmt.Errorf("store: scan user custom field: %w", err)
		}
		if err := json.Unmarshal([]byte(options), &f.Options); err != nil {
			return nil, fmt.Errorf("store: decode user custom field options: %w", err)
		}
		f.VisibleToEveryone = visible != 0
		f.CreatedAt, f.UpdatedAt = Time(created), Time(updated)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read user custom fields: %w", err)
	}
	return out, nil
}
