package store

import (
	"context"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
)

func GetGeneralMaster(ctx context.Context, q Querier, id string) (model.GeneralMaster, error) {
	var x model.GeneralMaster
	var d pf08SQLNullString
	var a, b, c, cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,name,description,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description,created_at,updated_at FROM general_masters WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Name, &d, &x.DefaultSortBy, &a, &b, &c, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Description = d.ptr()
	x.InitialDisplayCode = a != 0
	x.InitialDisplayCreatedAt = b != 0
	x.InitialDisplayDescription = c != 0
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetGeneralMasterField(ctx context.Context, q Querier, id string) (model.GeneralMasterField, error) {
	var x model.GeneralMasterField
	var d pf08SQLNullString
	var opts pf08SQLNullString
	var req, visible, initial, cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,general_master_id,code,title,description,field_type,required,field_order,visible,initial_display,options,created_at,updated_at FROM general_master_fields WHERE id=?`, id).Scan(&x.ID, &x.GeneralMasterID, &x.Code, &x.Title, &d, &x.FieldType, &req, &x.FieldOrder, &visible, &initial, &opts, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Description = d.ptr()
	x.Required = req != 0
	x.Visible = visible != 0
	x.InitialDisplay = initial != 0
	if opts.valid {
		if err = json.Unmarshal([]byte(opts.s), &x.Options); err != nil {
			return x, fmt.Errorf("store: decode master field options: %w", err)
		}
	}
	if x.Options == nil {
		x.Options = []string{}
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetGeneralMasterItem(ctx context.Context, q Querier, id string) (model.GeneralMasterItem, error) {
	var x model.GeneralMasterItem
	var d, starts, ends, inputs, inputIDs pf08SQLNullString
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,general_master_id,code,name,description,starts_on,ends_on,inputs,input_ids,created_at,updated_at FROM general_master_items WHERE id=?`, id).Scan(&x.ID, &x.GeneralMasterID, &x.Code, &x.Name, &d, &starts, &ends, &inputs, &inputIDs, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Description = d.ptr()
	x.StartsOn = starts.ptr()
	x.EndsOn = ends.ptr()
	if inputs.valid {
		if err = json.Unmarshal([]byte(inputs.s), &x.Inputs); err != nil {
			return x, fmt.Errorf("store: decode master item inputs: %w", err)
		}
	}
	if x.Inputs == nil {
		x.Inputs = map[string]any{}
	}
	if inputIDs.valid {
		if err = json.Unmarshal([]byte(inputIDs.s), &x.InputIDs); err != nil {
			return x, fmt.Errorf("store: decode master item input IDs: %w", err)
		}
	}
	if x.InputIDs == nil {
		x.InputIDs = map[string]string{}
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetGeneralMasterItems(ctx context.Context, q Querier, ids []string) ([]model.GeneralMasterItem, error) {
	out := make([]model.GeneralMasterItem, 0, len(ids))
	for _, id := range ids {
		x, e := GetGeneralMasterItem(ctx, q, id)
		if e == nil {
			out = append(out, x)
		} else if e != ErrNotFound {
			return nil, e
		}
	}
	return out, nil
}
func CountMasterItems(ctx context.Context, q Querier, id string) (int, error) {
	return pf08Count(ctx, q, `SELECT count(*) FROM general_master_items WHERE general_master_id=?`, id)
}
func MasterItemIDByCode(ctx context.Context, q Querier, masterID, code string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM general_master_items WHERE general_master_id=? AND code=?`, masterID, code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}

func GetGeneralMasterFields(ctx context.Context, q Querier, masterID string) ([]model.GeneralMasterField, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,general_master_id,code,title,description,field_type,required,field_order,visible,initial_display,options,created_at,updated_at FROM general_master_fields WHERE general_master_id=? ORDER BY field_order,id`, masterID)
	if err != nil {
		return nil, fmt.Errorf("store: query master fields: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.GeneralMasterField{}
	for rows.Next() {
		var x model.GeneralMasterField
		var d, opts pf08SQLNullString
		var required, visible, initial, cr, up int64
		if err = rows.Scan(&x.ID, &x.GeneralMasterID, &x.Code, &x.Title, &d, &x.FieldType, &required, &x.FieldOrder, &visible, &initial, &opts, &cr, &up); err != nil {
			return nil, err
		}
		x.Description = d.ptr()
		x.Required = required != 0
		x.Visible = visible != 0
		x.InitialDisplay = initial != 0
		if opts.valid {
			if err = json.Unmarshal([]byte(opts.s), &x.Options); err != nil {
				return nil, fmt.Errorf("store: decode master field options: %w", err)
			}
		}
		if x.Options == nil {
			x.Options = []string{}
		}
		x.CreatedAt = Time(cr)
		x.UpdatedAt = Time(up)
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate master fields: %w", err)
	}
	return out, nil
}
