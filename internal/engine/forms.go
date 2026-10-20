package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14NormalizeForm(ctx context.Context, q store.Querier, ticketID string, def model.WorkflowDef, inputs []FieldInput, slips []SlipItemInput, full bool, nowTime int64) ([]model.TicketInput, []model.TicketSlipItem, error) {
	fieldByID, fieldByCode := map[string]model.FormField{}, map[string]model.FormField{}
	slipByID, slipByCode := map[string]model.WorkflowSection{}, map[string]model.WorkflowSection{}
	for _, section := range def.SectionList {
		switch section.SectionType {
		case model.SectionTypeForm:
			for _, f := range section.FormFields {
				fieldByID[f.ID] = f
				fieldByCode[f.Code] = f
			}
		case model.SectionTypeSlip:
			slipByID[section.ID] = section
			slipByCode[section.Code] = section
		}
	}
	seen := map[string]bool{}
	normalized := []model.TicketInput{}
	errors := map[string][]string{}
	for _, in := range inputs {
		field, ok := model.FormField{}, false
		if in.FormFieldID != "" {
			field, ok = fieldByID[in.FormFieldID]
		} else if in.FormFieldCode != "" {
			field, ok = fieldByCode[in.FormFieldCode]
		}
		if !ok {
			return nil, nil, kferr.InvalidParameter("Invalid form field code")
		}
		if seen[field.ID] {
			return nil, nil, kferr.InvalidParameter("Form field must be unique")
		}
		seen[field.ID] = true
		value, refs, err := pf14FieldValue(in)
		if err != nil {
			errors[field.Code] = append(errors[field.Code], "is invalid")
			continue
		}
		if field.FieldType == "file" && len(in.Files) > 0 {
			value = in.Files
			refs = append([]string(nil), in.Files...)
		}
		if field.FieldType == "calculation" {
			value, refs = nil, nil
		}
		if err = pf14ValidateValue(field, value); err != nil {
			errors[field.Code] = append(errors[field.Code], err.Error())
			continue
		}
		if field.FieldType != "calculation" {
			if err = pf14cValidateFormValue(ctx, q, field, value); err != nil {
				errors[field.Code] = append(errors[field.Code], err.Error())
				continue
			}
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
			if full && field.Required && !field.Approver && !hidden {
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
			if in.SlipFieldID == "" {
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

func pf14cValidateFormValue(ctx context.Context, q store.Querier, field model.FormField, value any) error {
	if pf14Empty(value) {
		return nil
	}
	badType := fmt.Errorf("has an invalid value")
	stringsFromValue := func(v any) ([]string, bool) {
		if s, ok := v.(string); ok {
			return []string{s}, true
		}
		if values, ok := v.([]string); ok {
			return values, true
		}
		values, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(values))
		for _, item := range values {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	switch field.FieldType {
	case "text", "text_long":
		v, ok := value.(string)
		if !ok {
			return badType
		}
		if field.MinLength != nil && len([]rune(v)) < *field.MinLength || field.MaxLength != nil && len([]rune(v)) > *field.MaxLength {
			return fmt.Errorf("has an invalid length")
		}
		if field.RegexpFormat != nil && *field.RegexpFormat != "" {
			matched, err := regexp.MatchString(*field.RegexpFormat, v)
			if err != nil || !matched {
				return fmt.Errorf("has an invalid format")
			}
		}
	case "number", "integer":
		ok, n := pf14Float(value)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || field.FieldType == "integer" && math.Trunc(n) != n {
			return fmt.Errorf("must be a %s", field.FieldType)
		}
		if field.MinValue != nil && n < *field.MinValue || field.MaxValue != nil && n > *field.MaxValue {
			return fmt.Errorf("is outside the allowed range")
		}
		if field.DecimalDigit != nil {
			text := fmt.Sprint(value)
			if dot := strings.IndexByte(text, '.'); dot >= 0 && len(text)-dot-1 > *field.DecimalDigit {
				return fmt.Errorf("has too many decimal digits")
			}
		}
	case "checkbox":
		values, ok := stringsFromValue(value)
		if !ok {
			return badType
		}
		for _, v := range values {
			if !pf14cOptionAllowed(field.Options, v) {
				return fmt.Errorf("is not an available option")
			}
		}
	case "pull_down", "radio_button":
		v, ok := value.(string)
		if !ok || !pf14cOptionAllowed(field.Options, v) {
			return fmt.Errorf("is not an available option")
		}
	case "date", "datetime":
		v, ok := value.(string)
		layout := "2006-01-02"
		if field.FieldType == "datetime" {
			layout = "2006-01-02 15:04"
		}
		if !ok {
			return badType
		}
		if _, err := time.Parse(layout, v); err != nil {
			return fmt.Errorf("has an invalid date format")
		}
	case "master", "user", "team", "ticket", "file":
		ids, ok := stringsFromValue(value)
		if !ok || (!field.Multiple && len(ids) != 1) {
			return badType
		}
		for _, id := range ids {
			valid, err := pf14cReferenceExists(ctx, q, field, id)
			if err != nil {
				return err
			}
			if !valid {
				return fmt.Errorf("references an unknown or unavailable item")
			}
		}
	case "calculation", "button_api", "button_kintone":
		return nil
	default:
		return badType
	}
	return nil
}

func pf14cOptionAllowed(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func pf14cReferenceExists(ctx context.Context, q store.Querier, field model.FormField, id string) (bool, error) {
	switch field.FieldType {
	case "user":
		return store.EngUserExists(ctx, q, id)
	case "team":
		return store.EngTeamInActiveChart(ctx, q, id)
	case "ticket":
		status, exists, err := store.EngTicketStatus(ctx, q, id)
		if err != nil || !exists {
			return exists, err
		}
		if len(field.AllowedTicketStatus) == 0 {
			return true, nil
		}
		return pf14cOptionAllowed(field.AllowedTicketStatus, status), nil
	case "master":
		return store.EngMasterItemInMaster(ctx, q, id, field.GeneralMasterCode)
	case "file":
		return store.EngSignedFileExists(ctx, q, id)
	default:
		return false, nil
	}
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

func pf14ValidateApprovalInputs(def model.WorkflowDef, rows []model.TicketInput) error {
	inputs := pf14InputsFromRows(def, rows)
	values := pf14InputValues(def, inputs)
	for _, section := range def.SectionList {
		if section.SectionType != model.SectionTypeForm || !pf14SectionVisible(section, values) {
			continue
		}
		for _, field := range section.FormFields {
			if field.Approver && field.Required && pf14Empty(values[field.Code]) {
				return kferr.InputRequiredBeforeApprove()
			}
		}
	}
	return nil
}
