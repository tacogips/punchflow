package present

import (
	"context"
	"fmt"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"sort"
)

func GeneralMaster(ctx context.Context, q store.Querier, id string) (GeneralMasterJSON, error) {
	m, e := store.GetGeneralMaster(ctx, q, id)
	if e != nil {
		return nil, e
	}
	count, e := store.CountMasterItems(ctx, q, id)
	if e != nil {
		return nil, e
	}
	fields, e := store.GetGeneralMasterFields(ctx, q, id)
	if e != nil {
		return nil, e
	}
	f := []any{}
	for _, x := range fields {
		opts := []string{}
		if x.Options != nil {
			opts = x.Options
		}
		f = append(f, map[string]any{"id": x.ID, "code": x.Code, "title": x.Title, "description": x.Description, "fieldType": x.FieldType, "required": x.Required, "fieldOrder": x.FieldOrder, "visible": x.Visible, "initialDisplay": x.InitialDisplay, "options": opts, "createdAt": pf08Time(x.CreatedAt), "updatedAt": pf08Time(x.UpdatedAt)})
	}
	return GeneralMasterJSON{"id": m.ID, "code": m.Code, "name": m.Name, "description": m.Description, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "defaultSortBy": m.DefaultSortBy, "itemsCount": count, "initialDisplayCode": m.InitialDisplayCode, "initialDisplayCreatedAt": m.InitialDisplayCreatedAt, "initialDisplayDescription": m.InitialDisplayDescription, "fields": f}, nil
}
func GeneralMasterItem(ctx context.Context, q store.Querier, id string) (GeneralMasterItemJSON, error) {
	m, e := store.GetGeneralMasterItem(ctx, q, id)
	if e != nil {
		return nil, e
	}
	type itemInput struct {
		order int
		body  map[string]any
	}
	ordered := []itemInput{}
	for fieldID, value := range m.Inputs {
		inputID, ok := m.InputIDs[fieldID]
		if !ok || inputID == "" {
			return nil, fmt.Errorf("present: general master item %s: field %s has no input id", m.ID, fieldID)
		}
		field, er := store.GetGeneralMasterField(ctx, q, fieldID)
		if er != nil {
			return nil, er
		}
		ordered = append(ordered, itemInput{order: field.FieldOrder, body: map[string]any{"id": inputID, "value": value, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "field": pf08MasterField(field)}})
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].order < ordered[j].order })
	inputs := make([]any, 0, len(ordered))
	for _, input := range ordered {
		inputs = append(inputs, input.body)
	}
	return GeneralMasterItemJSON{"id": m.ID, "code": m.Code, "name": m.Name, "description": m.Description, "startsOn": m.StartsOn, "endsOn": m.EndsOn, "inputs": inputs, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}, nil
}
func GeneralMasterItems(ctx context.Context, q store.Querier, ids []string) ([]GeneralMasterItemJSON, error) {
	xs, e := store.GetGeneralMasterItems(ctx, q, ids)
	if e != nil {
		return nil, e
	}
	out := make([]GeneralMasterItemJSON, 0, len(xs))
	for _, x := range xs {
		j, e := GeneralMasterItem(ctx, q, x.ID)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}

func pf08MasterField(x model.GeneralMasterField) map[string]any {
	var options any
	if x.Options == nil {
		options = nil
	} else {
		options = x.Options
	}
	return map[string]any{"id": x.ID, "title": x.Title, "description": x.Description, "code": x.Code, "fieldType": x.FieldType, "required": x.Required, "fieldOrder": x.FieldOrder, "visible": x.Visible, "initialDisplay": x.InitialDisplay, "options": options, "createdAt": pf08Time(x.CreatedAt), "updatedAt": pf08Time(x.UpdatedAt)}
}
