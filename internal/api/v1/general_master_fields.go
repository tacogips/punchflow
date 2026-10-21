package v1

import (
	"context"
	"strings"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf20MasterFieldPatch is one entry of PATCH /v1/generalMasters fields. Only
// code is required; every other property keeps the stored value when absent.
type pf20MasterFieldPatch struct {
	Code           string             `json:"code"`
	Title          *string            `json:"title"`
	Description    nullable[string]   `json:"description"`
	FieldType      *string            `json:"fieldType"`
	Required       *bool              `json:"required"`
	Visible        *bool              `json:"visible"`
	InitialDisplay *bool              `json:"initialDisplay"`
	Options        nullable[[]string] `json:"options"`
}

// pf20MasterPatch is the PATCH body of a general master: the shared scalar
// properties plus presence-aware custom field entries.
type pf20MasterPatch struct {
	pf20MasterBody
	Fields *[]pf20MasterFieldPatch `json:"fields"`
}

// pf20MergeMasterFields resolves each patch entry by field code and merges the
// explicitly supplied properties over the stored field. Unknown codes create a
// new field, which then needs a title and fieldType. The resulting slice follows
// the request order and defines the new field set: stored fields that are not
// listed are removed, matching the existing replace-the-list behavior.
func pf20MergeMasterFields(ctx context.Context, q store.Querier, master string, patches []pf20MasterFieldPatch, now time.Time) ([]model.GeneralMasterField, error) {
	stored, err := store.GetGeneralMasterFields(ctx, q, master)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]model.GeneralMasterField, len(stored))
	for _, field := range stored {
		byCode[field.Code] = field
	}
	invalid := func(message string) error {
		return kferr.ValidationFailed(map[string][]string{"fields": {message}})
	}
	out := make([]model.GeneralMasterField, 0, len(patches))
	seen := map[string]bool{}
	for i, patch := range patches {
		code := strings.TrimSpace(patch.Code)
		if code == "" || seen[code] {
			return nil, invalid("contains an invalid or duplicate field")
		}
		seen[code] = true
		field, exists := byCode[code]
		if !exists {
			field = model.GeneralMasterField{ID: store.NewID(), GeneralMasterID: master, Code: code, Visible: true, CreatedAt: now}
			if patch.Title == nil || patch.FieldType == nil {
				return nil, invalid("title and fieldType are required for a new field")
			}
		}
		if patch.Title != nil {
			field.Title = strings.TrimSpace(*patch.Title)
		}
		if patch.FieldType != nil {
			if exists && *patch.FieldType != field.FieldType {
				return nil, invalid("fieldType of an existing field cannot be changed")
			}
			field.FieldType = *patch.FieldType
		}
		patch.Description.applyTo(&field.Description)
		if patch.Required != nil {
			field.Required = *patch.Required
		}
		if patch.Visible != nil {
			field.Visible = *patch.Visible
		}
		if patch.InitialDisplay != nil {
			field.InitialDisplay = *patch.InitialDisplay
		}
		if patch.Options.Set {
			field.Options = patch.Options.Value
		}
		if field.Title == "" || !pf20Contains(model.MasterFieldTypes, field.FieldType) {
			return nil, invalid("contains an invalid or duplicate field")
		}
		if (field.FieldType == model.MasterFieldTypeCheckbox || field.FieldType == model.MasterFieldTypePullDown) && len(field.Options) == 0 {
			return nil, invalid("options are required for selection fields")
		}
		field.FieldOrder = i
		field.UpdatedAt = now
		out = append(out, field)
	}
	return out, nil
}
