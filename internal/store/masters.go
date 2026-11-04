package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"punchflow/internal/model"
	"sort"
)

// GeneralMasterList returns a page of general masters, optionally limited to
// the masters admitted by filter.
func GeneralMasterList(ctx context.Context, q Querier, offset, limit int, sortBy string, desc bool, filter IDFilter) ([]model.GeneralMaster, int, error) {
	scope, scopeArgs := filter.clause("id")
	order := map[string]string{"createdAt": "created_at", "code": "code", "name": "name"}[sortBy]
	if order == "" {
		order = "name"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM general_masters WHERE 1=1`+scope, scopeArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM general_masters WHERE 1=1`+scope+` ORDER BY `+order+`,id LIMIT ? OFFSET ?`, append(append([]any{}, scopeArgs...), limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	out := make([]model.GeneralMaster, 0, len(ids))
	for _, id := range ids {
		x, e := GetGeneralMaster(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	return out, total, nil
}
func GeneralMasterItemsList(ctx context.Context, q Querier, masterID string, offset, limit int, sortBy string, desc bool) ([]model.GeneralMasterItem, int, error) {
	order := map[string]string{"createdAt": "created_at", "code": "code", "name": "name"}[sortBy]
	if order == "" {
		order = "code"
	}
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM general_master_items WHERE general_master_id=?`, masterID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.QueryContext(ctx, `SELECT id FROM general_master_items WHERE general_master_id=? ORDER BY `+order+`,id LIMIT ? OFFSET ?`, masterID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	out := make([]model.GeneralMasterItem, 0, len(ids))
	for _, id := range ids {
		x, e := GetGeneralMasterItem(ctx, q, id)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, x)
	}
	return out, total, nil
}
func CreateGeneralMaster(ctx context.Context, d *DB, x model.GeneralMaster, fields []model.GeneralMasterField) error {
	return d.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO general_masters(id,code,name,description,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?)`, x.ID, x.Code, x.Name, x.Description, x.DefaultSortBy, x.InitialDisplayCode, x.InitialDisplayCreatedAt, x.InitialDisplayDescription, Millis(x.CreatedAt), Millis(x.UpdatedAt)); err != nil {
			return err
		}
		return pf20WriteMasterFields(ctx, tx, x.ID, fields)
	})
}
func UpdateGeneralMaster(ctx context.Context, d *DB, x model.GeneralMaster, fields *[]model.GeneralMasterField) error {
	return d.WithTx(ctx, func(tx *Tx) error {
		var oldCode string
		if err := tx.QueryRowContext(ctx, `SELECT code FROM general_masters WHERE id=?`, x.ID).Scan(&oldCode); err != nil {
			return err
		}
		if err := ReconcileDefinitionCodeChange(ctx, tx, "master", oldCode, x.Code, x.UpdatedAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE general_masters SET code=?,name=?,description=?,default_sort_by=?,initial_display_code=?,initial_display_created_at=?,initial_display_description=?,updated_at=? WHERE id=?`, x.Code, x.Name, x.Description, x.DefaultSortBy, x.InitialDisplayCode, x.InitialDisplayCreatedAt, x.InitialDisplayDescription, Millis(x.UpdatedAt), x.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		if fields != nil {
			return pf20WriteMasterFields(ctx, tx, x.ID, *fields)
		}
		return nil
	})
}
func pf20WriteMasterFields(ctx context.Context, q Querier, masterID string, fields []model.GeneralMasterField) error {
	old := map[string]string{}
	rows, err := q.QueryContext(ctx, `SELECT id,code FROM general_master_fields WHERE general_master_id=?`, masterID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, code string
		if err = rows.Scan(&id, &code); err != nil {
			_ = rows.Close()
			return err
		}
		old[code] = id
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	keep := map[string]bool{}
	for _, f := range fields {
		id := f.ID
		if existing := old[f.Code]; existing != "" {
			id = existing
		}
		if id == "" {
			id = NewID()
		}
		keep[id] = true
		var options any
		if f.Options != nil {
			b, e := json.Marshal(f.Options)
			if e != nil {
				return e
			}
			options = string(b)
		}
		if existing := old[f.Code]; existing != "" {
			_, err = q.ExecContext(ctx, `UPDATE general_master_fields SET title=?,description=?,field_type=?,required=?,field_order=?,visible=?,initial_display=?,options=?,updated_at=? WHERE id=?`, f.Title, f.Description, f.FieldType, f.Required, f.FieldOrder, f.Visible, f.InitialDisplay, options, Millis(f.UpdatedAt), id)
		} else {
			_, err = q.ExecContext(ctx, `INSERT INTO general_master_fields(id,general_master_id,code,title,description,field_type,required,field_order,visible,initial_display,options,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, masterID, f.Code, f.Title, f.Description, f.FieldType, f.Required, f.FieldOrder, f.Visible, f.InitialDisplay, options, Millis(f.CreatedAt), Millis(f.UpdatedAt))
		}
		if err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(old))
	for _, id := range old {
		if !keep[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err = pf20RemoveDeletedFieldInputs(ctx, q, masterID, id); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, `DELETE FROM general_master_fields WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

// pf20RemoveDeletedFieldInputs removes values and stable input IDs that point
// at a field being removed. It runs inside UpdateGeneralMaster's transaction.
func pf20RemoveDeletedFieldInputs(ctx context.Context, q Querier, masterID, fieldID string) error {
	rows, err := q.QueryContext(ctx, `SELECT id,inputs,input_ids FROM general_master_items WHERE general_master_id=?`, masterID)
	if err != nil {
		return err
	}
	type itemState struct{ id, inputs, inputIDs string }
	items := []itemState{}
	for rows.Next() {
		var item itemState
		if err = rows.Scan(&item.id, &item.inputs, &item.inputIDs); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		inputs := map[string]any{}
		if err = json.Unmarshal([]byte(item.inputs), &inputs); err != nil {
			return err
		}
		inputIDs := map[string]string{}
		if err = json.Unmarshal([]byte(item.inputIDs), &inputIDs); err != nil {
			return err
		}
		if _, exists := inputs[fieldID]; !exists {
			if _, exists = inputIDs[fieldID]; !exists {
				continue
			}
		}
		delete(inputs, fieldID)
		delete(inputIDs, fieldID)
		inputJSON, marshalErr := json.Marshal(inputs)
		if marshalErr != nil {
			return marshalErr
		}
		idsJSON, marshalErr := json.Marshal(inputIDs)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = q.ExecContext(ctx, `UPDATE general_master_items SET inputs=?,input_ids=? WHERE id=?`, string(inputJSON), string(idsJSON), item.id); err != nil {
			return err
		}
	}
	return nil
}
func DeleteGeneralMaster(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM general_masters WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func CreateGeneralMasterItem(ctx context.Context, q Querier, x model.GeneralMasterItem) error {
	inputs, err := json.Marshal(x.Inputs)
	if err != nil {
		return err
	}
	ids, err := json.Marshal(x.InputIDs)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO general_master_items(id,general_master_id,code,name,description,starts_on,ends_on,inputs,input_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.GeneralMasterID, x.Code, x.Name, x.Description, x.StartsOn, x.EndsOn, string(inputs), string(ids), Millis(x.CreatedAt), Millis(x.UpdatedAt))
	return err
}
func UpdateGeneralMasterItem(ctx context.Context, q Querier, x model.GeneralMasterItem) error {
	var oldCode string
	if err := q.QueryRowContext(ctx, `SELECT code FROM general_master_items WHERE id=? AND general_master_id=?`, x.ID, x.GeneralMasterID).Scan(&oldCode); err != nil {
		return err
	}
	master, err := GetGeneralMaster(ctx, q, x.GeneralMasterID)
	if err != nil {
		return err
	}
	if err := ReconcileDefinitionCodeChange(ctx, q, "master_item", oldCode, x.Code, x.UpdatedAt, master.Code); err != nil {
		return err
	}
	inputs, err := json.Marshal(x.Inputs)
	if err != nil {
		return err
	}
	ids, err := json.Marshal(x.InputIDs)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, `UPDATE general_master_items SET code=?,name=?,description=?,starts_on=?,ends_on=?,inputs=?,input_ids=?,updated_at=? WHERE id=? AND general_master_id=?`, x.Code, x.Name, x.Description, x.StartsOn, x.EndsOn, string(inputs), string(ids), Millis(x.UpdatedAt), x.ID, x.GeneralMasterID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func DeleteGeneralMasterItem(ctx context.Context, q Querier, masterID, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM general_master_items WHERE id=? AND general_master_id=?`, id, masterID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func GeneralMasterItemBelongsTo(ctx context.Context, q Querier, masterID, itemID string) bool {
	var id string
	return q.QueryRowContext(ctx, `SELECT id FROM general_master_items WHERE id=? AND general_master_id=?`, itemID, masterID).Scan(&id) == nil
}
func GeneralMasterFieldByCode(ctx context.Context, q Querier, masterID, code string) (model.GeneralMasterField, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM general_master_fields WHERE general_master_id=? AND code=?`, masterID, code).Scan(&id)
	if err == sql.ErrNoRows {
		return model.GeneralMasterField{}, ErrNotFound
	}
	if err != nil {
		return model.GeneralMasterField{}, err
	}
	return GetGeneralMasterField(ctx, q, id)
}
