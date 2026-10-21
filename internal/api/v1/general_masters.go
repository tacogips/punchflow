package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf20MasterFieldBody struct {
	Code           string   `json:"code"`
	Title          string   `json:"title"`
	Description    *string  `json:"description"`
	FieldType      string   `json:"fieldType"`
	Required       bool     `json:"required"`
	Visible        *bool    `json:"visible"`
	InitialDisplay bool     `json:"initialDisplay"`
	Options        []string `json:"options"`
}
type pf20MasterBody struct {
	Code                      *string                `json:"code"`
	Name                      *string                `json:"name"`
	Description               *string                `json:"description"`
	DefaultSortBy             *string                `json:"defaultSortBy"`
	InitialDisplayCode        *bool                  `json:"initialDisplayCode"`
	InitialDisplayCreatedAt   *bool                  `json:"initialDisplayCreatedAt"`
	InitialDisplayDescription *bool                  `json:"initialDisplayDescription"`
	Fields                    *[]pf20MasterFieldBody `json:"fields"`
}
type pf20InputBody struct {
	Code  string          `json:"code"`
	Value json.RawMessage `json:"value"`
}
type pf20ItemBody struct {
	Code        *string          `json:"code"`
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	StartsOn    nullable[string] `json:"startsOn"`
	EndsOn      nullable[string] `json:"endsOn"`
	Inputs      *[]pf20InputBody `json:"inputs"`
}

func (a *API) listGeneralMasters(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMasterRead, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	p, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	s := httpx.ParseSort(r, "createdAt", "code", "name")
	by := s.Field
	if by == "" {
		by = "name"
	}
	xs, total, e := store.GeneralMasterList(r.Context(), a.d.DB.Q(), p.Offset(), p.PerPage, by, s.Desc, masterListFilter(principalOf(r)))
	if e != nil {
		kferr.Write(w, e)
		return
	}
	out := make([]present.GeneralMasterJSON, 0, len(xs))
	for _, x := range xs {
		j, er := present.GeneralMaster(r.Context(), a.d.DB.Q(), x.ID)
		if er != nil {
			kferr.Write(w, er)
			return
		}
		out = append(out, j)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createGeneralMaster(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	if e := requireUnrestricted(principalOf(r), model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	var b pf20MasterBody
	if e := httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Name == nil || strings.TrimSpace(*b.Name) == "" {
		kferr.Write(w, kferr.InvalidParameter("name is required"))
		return
	}
	if err := pf20ValidateLength("name", strings.TrimSpace(*b.Name), 300); err != nil {
		kferr.Write(w, err)
		return
	}
	x := model.GeneralMaster{ID: store.NewID(), Code: "master-" + store.NewID()[:8], Name: strings.TrimSpace(*b.Name), DefaultSortBy: model.MasterSortKeyName, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if b.Code != nil && strings.TrimSpace(*b.Code) != "" {
		x.Code = strings.TrimSpace(*b.Code)
	}
	if err := pf20ValidateLength("code", x.Code, 100); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.Description != nil {
		if err := pf20ValidateLength("description", *b.Description, 10000); err != nil {
			kferr.Write(w, err)
			return
		}
		x.Description = b.Description
	}
	if b.DefaultSortBy != nil {
		x.DefaultSortBy = *b.DefaultSortBy
	}
	if !pf20Contains(model.MasterSortKeys, x.DefaultSortBy) {
		kferr.Write(w, kferr.InvalidParameter("Invalid defaultSortBy"))
		return
	}
	if b.InitialDisplayCode != nil {
		x.InitialDisplayCode = *b.InitialDisplayCode
	}
	if b.InitialDisplayCreatedAt != nil {
		x.InitialDisplayCreatedAt = *b.InitialDisplayCreatedAt
	}
	if b.InitialDisplayDescription != nil {
		x.InitialDisplayDescription = *b.InitialDisplayDescription
	}
	fields, e := pf20MasterFields(b.Fields, x.ID, x.CreatedAt)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if e = store.CreateGeneralMaster(r.Context(), a.d.DB, x, fields); e != nil {
		if pf20IsUniqueViolation(e) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, e)
		}
		return
	}
	j, e := present.GeneralMaster(r.Context(), a.d.DB.Q(), x.ID)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) getGeneralMaster(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMasterRead, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, id, false) {
		return
	}
	j, e := present.GeneralMaster(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) updateGeneralMaster(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, id, true) {
		return
	}
	x, e := store.GetGeneralMaster(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	var b pf20MasterPatch
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Code != nil && strings.TrimSpace(*b.Code) != "" {
		x.Code = strings.TrimSpace(*b.Code)
	}
	if b.Name != nil {
		x.Name = strings.TrimSpace(*b.Name)
	}
	if x.Name == "" {
		kferr.Write(w, kferr.InvalidParameter("name is required"))
		return
	}
	if err := pf20ValidateLength("name", x.Name, 300); err != nil {
		kferr.Write(w, err)
		return
	}
	if err := pf20ValidateLength("code", x.Code, 100); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.Description != nil {
		if err := pf20ValidateLength("description", *b.Description, 10000); err != nil {
			kferr.Write(w, err)
			return
		}
		x.Description = b.Description
	}
	if b.DefaultSortBy != nil {
		x.DefaultSortBy = *b.DefaultSortBy
	}
	if !pf20Contains(model.MasterSortKeys, x.DefaultSortBy) {
		kferr.Write(w, kferr.InvalidParameter("Invalid defaultSortBy"))
		return
	}
	if b.InitialDisplayCode != nil {
		x.InitialDisplayCode = *b.InitialDisplayCode
	}
	if b.InitialDisplayCreatedAt != nil {
		x.InitialDisplayCreatedAt = *b.InitialDisplayCreatedAt
	}
	if b.InitialDisplayDescription != nil {
		x.InitialDisplayDescription = *b.InitialDisplayDescription
	}
	x.UpdatedAt = time.Now().UTC()
	var fs *[]model.GeneralMasterField
	if b.Fields != nil {
		converted, e := pf20MergeMasterFields(r.Context(), a.d.DB.Q(), id, *b.Fields, x.UpdatedAt)
		if e != nil {
			kferr.Write(w, e)
			return
		}
		fs = &converted
	}
	if e = store.UpdateGeneralMaster(r.Context(), a.d.DB, x, fs); e != nil {
		if pf20IsUniqueViolation(e) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, e)
		}
		return
	}
	j, e := present.GeneralMaster(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) deleteGeneralMaster(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, id, true) {
		return
	}
	if e = store.DeleteGeneralMaster(r.Context(), a.d.DB.Q(), id); e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) listGeneralMasterItems(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMasterRead, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	master, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, master, false) {
		return
	}
	p, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	s := httpx.ParseSort(r, "createdAt", "code", "name")
	by := s.Field
	if by == "" {
		by = ""
	}
	if by == "" {
		m, er := store.GetGeneralMaster(r.Context(), a.d.DB.Q(), master)
		if er != nil {
			kferr.Write(w, er)
			return
		}
		by = m.DefaultSortBy
	}
	xs, total, e := store.GeneralMasterItemsList(r.Context(), a.d.DB.Q(), master, p.Offset(), p.PerPage, by, s.Desc)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	ids := make([]string, len(xs))
	for i, x := range xs {
		ids[i] = x.ID
	}
	out, e := present.GeneralMasterItems(r.Context(), a.d.DB.Q(), ids)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, p, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createGeneralMasterItem(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	master, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, master, true) {
		return
	}
	var b pf20ItemBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Name == nil || strings.TrimSpace(*b.Name) == "" || b.Inputs == nil {
		kferr.Write(w, kferr.InvalidParameter("name and inputs are required"))
		return
	}
	if err := pf20ValidateLength("name", strings.TrimSpace(*b.Name), 100); err != nil {
		kferr.Write(w, err)
		return
	}
	now := time.Now().UTC()
	x := model.GeneralMasterItem{ID: store.NewID(), GeneralMasterID: master, Code: "item-" + store.NewID()[:8], Name: strings.TrimSpace(*b.Name), CreatedAt: now, UpdatedAt: now}
	if b.Code != nil && strings.TrimSpace(*b.Code) != "" {
		x.Code = strings.TrimSpace(*b.Code)
	}
	if err := pf20ValidateLength("code", x.Code, 100); err != nil {
		kferr.Write(w, err)
		return
	}
	x.Description = b.Description
	x.StartsOn = b.StartsOn.pointer()
	x.EndsOn = b.EndsOn.pointer()
	if e = pf20ValidateDateBounds(x.StartsOn, x.EndsOn); e != nil {
		kferr.Write(w, e)
		return
	}
	x.Inputs, x.InputIDs, e = pf20BuildInputs(r.Context(), a.d.DB.Q(), master, *b.Inputs, nil, nil)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if e = store.CreateGeneralMasterItem(r.Context(), a.d.DB.Q(), x); e != nil {
		if pf20IsUniqueViolation(e) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, e)
		}
		return
	}
	j, e := present.GeneralMasterItem(r.Context(), a.d.DB.Q(), x.ID)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) getGeneralMasterItem(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMasterRead, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	master, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, master, false) {
		return
	}
	raw := r.PathValue("itemId")
	item, e := pf20ResolveItem(r.Context(), a.d.DB.Q(), master, raw)
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	j, e := present.GeneralMasterItem(r.Context(), a.d.DB.Q(), item)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}
func (a *API) updateGeneralMasterItem(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	master, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, master, true) {
		return
	}
	id, e := pf20ResolveItem(r.Context(), a.d.DB.Q(), master, r.PathValue("itemId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	x, e := store.GetGeneralMasterItem(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	var b pf20ItemBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Code != nil && strings.TrimSpace(*b.Code) != "" {
		x.Code = strings.TrimSpace(*b.Code)
	}
	if b.Name != nil {
		x.Name = strings.TrimSpace(*b.Name)
	}
	if x.Name == "" {
		kferr.Write(w, kferr.InvalidParameter("name is required"))
		return
	}
	if err := pf20ValidateLength("name", x.Name, 100); err != nil {
		kferr.Write(w, err)
		return
	}
	if err := pf20ValidateLength("code", x.Code, 100); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.Description != nil {
		x.Description = b.Description
	}
	b.StartsOn.applyTo(&x.StartsOn)
	b.EndsOn.applyTo(&x.EndsOn)
	if e = pf20ValidateDateBounds(x.StartsOn, x.EndsOn); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Inputs != nil {
		x.Inputs, x.InputIDs, e = pf20BuildInputs(r.Context(), a.d.DB.Q(), master, *b.Inputs, x.Inputs, x.InputIDs)
		if e != nil {
			kferr.Write(w, e)
			return
		}
	}
	x.UpdatedAt = time.Now().UTC()
	if e = store.UpdateGeneralMasterItem(r.Context(), a.d.DB.Q(), x); e != nil {
		if pf20IsUniqueViolation(e) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}}))
		} else {
			kferr.Write(w, e)
		}
		return
	}
	j, e := present.GeneralMasterItem(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, j)
}

func pf20ValidateLength(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return kferr.ValidationFailed(map[string][]string{field: {"is too long (maximum is " + strconv.Itoa(max) + " characters)"}})
	}
	return nil
}
func (a *API) deleteGeneralMasterItem(w http.ResponseWriter, r *http.Request) {
	if e := auth.Require(r, model.PermissionMaster); e != nil {
		kferr.Write(w, e)
		return
	}
	master, e := store.ResolveID(r.Context(), a.d.DB.Q(), "general_masters", r.PathValue("generalMasterId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if !a.guardMaster(w, r, master, true) {
		return
	}
	id, e := pf20ResolveItem(r.Context(), a.d.DB.Q(), master, r.PathValue("itemId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if e = store.DeleteGeneralMasterItem(r.Context(), a.d.DB.Q(), master, id); e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteEmpty(w)
}

func pf20MasterFields(in *[]pf20MasterFieldBody, master string, now time.Time) ([]model.GeneralMasterField, error) {
	out := []model.GeneralMasterField{}
	if in == nil {
		return out, nil
	}
	seen := map[string]bool{}
	for i, f := range *in {
		f.Code = strings.TrimSpace(f.Code)
		f.Title = strings.TrimSpace(f.Title)
		if f.Code == "" || f.Title == "" || seen[f.Code] || !pf20Contains(model.MasterFieldTypes, f.FieldType) {
			return nil, kferr.ValidationFailed(map[string][]string{"fields": {"contains an invalid or duplicate field"}})
		}
		if err := pf20ValidateLength("fields", f.Code, 100); err != nil {
			return nil, err
		}
		if err := pf20ValidateLength("fields", f.Title, 300); err != nil {
			return nil, err
		}
		if f.Description != nil {
			if err := pf20ValidateLength("fields", *f.Description, 10000); err != nil {
				return nil, err
			}
		}
		seen[f.Code] = true
		if (f.FieldType == model.MasterFieldTypeCheckbox || f.FieldType == model.MasterFieldTypePullDown) && len(f.Options) == 0 {
			return nil, kferr.ValidationFailed(map[string][]string{"fields": {"options are required for selection fields"}})
		}
		visible := true
		if f.Visible != nil {
			visible = *f.Visible
		}
		out = append(out, model.GeneralMasterField{ID: store.NewID(), GeneralMasterID: master, Code: f.Code, Title: f.Title, Description: f.Description, FieldType: f.FieldType, Required: f.Required, FieldOrder: i, Visible: visible, InitialDisplay: f.InitialDisplay, Options: f.Options, CreatedAt: now, UpdatedAt: now})
	}
	return out, nil
}
func pf20BuildInputs(ctx context.Context, q store.Querier, master string, items []pf20InputBody, existing map[string]any, oldIDs map[string]string) (map[string]any, map[string]string, error) {
	fields, err := store.GetGeneralMasterFields(ctx, q, master)
	if err != nil {
		return nil, nil, err
	}
	byCode := map[string]model.GeneralMasterField{}
	for _, f := range fields {
		byCode[f.Code] = f
	}
	values := map[string]any{}
	ids := map[string]string{}
	seen := map[string]bool{}
	for _, item := range items {
		field, ok := byCode[item.Code]
		if !ok {
			return nil, nil, kferr.ValidationFailed(map[string][]string{"inputs": {"contains an unknown field code"}})
		}
		if seen[field.ID] {
			return nil, nil, kferr.ValidationFailed(map[string][]string{"inputs": {"contains a duplicate field"}})
		}
		seen[field.ID] = true
		var value any
		if err = json.Unmarshal(item.Value, &value); err != nil {
			return nil, nil, kferr.InvalidParameter("Invalid input value")
		}
		if err = pf20ValidateMasterValue(field, value); err != nil {
			return nil, nil, err
		}
		values[field.ID] = value
		if old := oldIDs[field.ID]; old != "" {
			ids[field.ID] = old
		} else {
			ids[field.ID] = store.NewID()
		}
	}
	for _, field := range fields {
		if field.Required {
			v, ok := values[field.ID]
			if !ok || v == nil || v == "" {
				return nil, nil, kferr.ValidationFailed(map[string][]string{"inputs": {"required field " + field.Code + " is missing"}})
			}
		}
	}
	if len(items) == 0 && len(existing) > 0 {
		return nil, nil, kferr.InvalidParameter("inputs must contain custom field values")
	}
	return values, ids, nil
}
func pf20ValidateMasterValue(f model.GeneralMasterField, v any) error {
	invalid := func() error {
		return kferr.ValidationFailed(map[string][]string{"inputs": {"has an invalid value for field " + f.Code}})
	}
	if v == nil {
		if f.Required {
			return invalid()
		}
		return nil
	}
	switch f.FieldType {
	case model.MasterFieldTypeCheckbox:
		xs, ok := v.([]any)
		if !ok {
			return invalid()
		}
		for _, raw := range xs {
			value, ok := raw.(string)
			if !ok || !pf20Contains(f.Options, value) {
				return invalid()
			}
		}
	case model.MasterFieldTypePullDown:
		value, ok := v.(string)
		if !ok || !pf20Contains(f.Options, value) {
			return invalid()
		}
	case model.MasterFieldTypeNumber:
		value, ok := v.(string)
		if !ok {
			return invalid()
		}
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return invalid()
		}
	case model.MasterFieldTypeInteger:
		value, ok := v.(string)
		if !ok {
			return invalid()
		}
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return invalid()
		}
	case model.MasterFieldTypeDate:
		value, ok := v.(string)
		if !ok {
			return invalid()
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return invalid()
		}
	default:
		if _, ok := v.(string); !ok {
			return invalid()
		}
	}
	return nil
}
func pf20ResolveItem(ctx context.Context, q store.Querier, master, raw string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM general_master_items WHERE general_master_id=? AND (id=? OR code=?) LIMIT 1`, master, raw, raw).Scan(&id)
	if err == sql.ErrNoRows {
		return "", store.ErrNotFound
	}
	return id, err
}
func pf20ValidateDateBounds(start, end *string) error {
	for _, v := range []*string{start, end} {
		if v != nil {
			if _, e := time.Parse("2006-01-02", *v); e != nil {
				return kferr.ValidationFailed(map[string][]string{"startsOn": {"must be an ISO date"}})
			}
		}
	}
	if start != nil && end != nil && *start > *end {
		return kferr.ValidationFailed(map[string][]string{"startsOn": {"must not be after endsOn"}})
	}
	return nil
}
func pf20Contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func pf20IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
