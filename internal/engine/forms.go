package engine

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

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
		// The default fixtures list "all" (not in the kickflow enum) to mean any status.
		if len(field.AllowedTicketStatus) == 0 || pf14cOptionAllowed(field.AllowedTicketStatus, "all") {
			return true, nil
		}
		return pf14cOptionAllowed(field.AllowedTicketStatus, status), nil
	case "master":
		return store.EngMasterItemInMaster(ctx, q, id, field.GeneralMasterCode)
	case "file":
		file, err := store.FileBySignedID(ctx, q, id)
		if err != nil {
			if err == store.ErrNotFound {
				return false, nil
			}
			return false, err
		}
		if len(field.AllowedExtensions) == 0 {
			return true, nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")
		return pf14cOptionAllowed(field.AllowedExtensions, ext), nil
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
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return pf14bEmptyArray(v)
}

// pf14ValidateApprovalInputs requires the approver-required form fields and the
// approver-required cells of every detail (slip) row to be filled before an
// approval. Hidden and calculated detail cells cannot be entered and are exempt.
func pf14ValidateApprovalInputs(def model.WorkflowDef, rows []model.TicketInput, slips []model.TicketSlipItem) error {
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
	sections := map[string]model.WorkflowSection{}
	for _, section := range def.SectionList {
		if section.SectionType == model.SectionTypeSlip {
			sections[section.ID] = section
		}
	}
	for _, row := range slips {
		section, ok := sections[row.SlipSectionID]
		if !ok {
			continue
		}
		for _, field := range section.SlipFields {
			hidden := field.Hidden != nil && *field.Hidden
			if !field.Approver || !field.Required || hidden || field.FieldType == "calculation" {
				continue
			}
			if cell, _ := slipCell(row, field); pf14Empty(cell) {
				return kferr.InputRequiredBeforeApprove()
			}
		}
	}
	return nil
}
