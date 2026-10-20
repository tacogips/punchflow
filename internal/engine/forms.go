package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14NormalizeForm(ctx context.Context, q store.Querier, ticketID string, def model.WorkflowDef, inputs []FieldInput, slips []SlipItemInput, full bool, nowTime int64) ([]model.TicketInput, []model.TicketSlipItem, error) {
	fieldByID, fieldByCode := map[string]model.FormField{}, map[string]model.FormField{}
	slipByID, slipByCode := map[string]model.WorkflowSection{}, map[string]model.WorkflowSection{}
	for _, section := range def.SectionList {
		if section.SectionType == model.SectionTypeForm {
			for _, f := range section.FormFields {
				fieldByID[f.ID] = f
				fieldByCode[f.Code] = f
			}
		} else if section.SectionType == model.SectionTypeSlip {
			slipByID[section.ID] = section
			slipByCode[section.Code] = section
		}
	}
	seen := map[string]bool{}
	normalized := []model.TicketInput{}
	errors := map[string][]string{}
	for _, in := range inputs {
		field, ok := fieldByID[in.FormFieldID]
		if !ok && in.FormFieldCode != "" {
			field, ok = fieldByCode[in.FormFieldCode]
		}
		if !ok {
			errors["inputs"] = append(errors["inputs"], "contains an unknown form field")
			continue
		}
		if seen[field.ID] {
			errors[field.Code] = append(errors[field.Code], "must be unique")
			continue
		}
		seen[field.ID] = true
		value, refs, err := pf14FieldValue(in)
		if err != nil {
			errors[field.Code] = append(errors[field.Code], "is invalid")
			continue
		}
		if err = pf14ValidateValue(field, value); err != nil {
			errors[field.Code] = append(errors[field.Code], err.Error())
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: encode form value: %w", err)
		}
		var decoded any
		if err = json.Unmarshal(raw, &decoded); err != nil {
			return nil, nil, err
		}
		normalized = append(normalized, model.TicketInput{ID: store.NewID(), TicketID: ticketID, FormFieldID: field.ID, Value: decoded, RefIDs: refs})
	}
	values := pf14InputValues(def, inputs)
	for _, section := range def.SectionList {
		if section.SectionType != model.SectionTypeForm || !pf14SectionVisible(section, values) {
			continue
		}
		for _, field := range section.FormFields {
			hidden := field.Hidden != nil && *field.Hidden
			if full && field.Required && !hidden {
				if !seen[field.ID] || pf14Empty(values[field.Code]) {
					errors[field.Code] = append(errors[field.Code], "is required")
				}
			}
		}
	}
	if len(errors) > 0 {
		return nil, nil, kferr.ValidationFailed(errors)
	}
	normalizedSlips := []model.TicketSlipItem{}
	for i, item := range slips {
		section, ok := slipByID[item.SlipSectionID]
		if !ok && item.SlipSectionCode != "" {
			section, ok = slipByCode[item.SlipSectionCode]
		}
		if !ok {
			return nil, nil, kferr.ValidationFailed(map[string][]string{"slipItems": {"contains an unknown slip section"}})
		}
		mapInputs := map[string]any{}
		fieldByID := map[string]model.SlipField{}
		for _, f := range section.SlipFields {
			fieldByID[f.ID] = f
			fieldByID[f.Code] = f
		}
		for _, in := range item.Inputs {
			field, ok := fieldByID[in.SlipFieldID]
			if !ok {
				field, ok = fieldByID[in.SlipFieldCode]
			}
			if !ok {
				return nil, nil, kferr.ValidationFailed(map[string][]string{"slipItems": {"contains an unknown slip field"}})
			}
			raw := in.Value
			if len(raw) == 0 {
				raw = in.GeneralMasterItemID
			}
			if len(raw) == 0 {
				raw = in.UserID
			}
			if len(raw) == 0 {
				raw = in.TeamID
			}
			if len(raw) == 0 {
				raw = in.TicketID
			}
			var v any
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &v); err != nil {
					return nil, nil, kferr.ValidationFailed(map[string][]string{field.Code: {"is invalid"}})
				}
			}
			if field.Required && full && pf14Empty(v) {
				return nil, nil, kferr.ValidationFailed(map[string][]string{field.Code: {"is required"}})
			}
			mapInputs[field.ID] = v
		}
		normalizedSlips = append(normalizedSlips, model.TicketSlipItem{ID: store.NewID(), TicketID: ticketID, SlipSectionID: section.ID, Position: i, Inputs: mapInputs})
	}
	for i := range normalized {
		normalized[i].CreatedAt = store.Time(nowTime)
		normalized[i].UpdatedAt = normalized[i].CreatedAt
	}
	for i := range normalizedSlips {
		normalizedSlips[i].CreatedAt = store.Time(nowTime)
		normalizedSlips[i].UpdatedAt = normalizedSlips[i].CreatedAt
	}
	return normalized, normalizedSlips, nil
}

func pf14ValidateValue(field model.FormField, value any) error {
	if pf14Empty(value) {
		return nil
	}
	text := fmt.Sprint(value)
	if field.MinLength != nil && len([]rune(text)) < *field.MinLength {
		return fmt.Errorf("is too short")
	}
	if field.MaxLength != nil && len([]rune(text)) > *field.MaxLength {
		return fmt.Errorf("is too long")
	}
	if field.RegexpFormat != nil && *field.RegexpFormat != "" {
		ok, err := regexp.MatchString(*field.RegexpFormat, text)
		if err != nil || !ok {
			return fmt.Errorf("has an invalid format")
		}
	}
	if len(field.Options) > 0 && field.FieldType != "checkbox" {
		found := false
		for _, option := range field.Options {
			if option == text {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("is not an available option")
		}
	}
	if field.MinValue != nil || field.MaxValue != nil {
		ok, n := pf14Float(value)
		if !ok {
			return fmt.Errorf("must be a number")
		}
		if field.MinValue != nil && n < *field.MinValue {
			return fmt.Errorf("is below the minimum")
		}
		if field.MaxValue != nil && n > *field.MaxValue {
			return fmt.Errorf("is above the maximum")
		}
	}
	return nil
}
func pf14Empty(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}
