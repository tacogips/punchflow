package engine

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"regexp"
	"strconv"
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
		maxLength := 300
		if field.FieldType == "text_long" {
			maxLength = 10000
		}
		if field.MaxLength != nil && *field.MaxLength < maxLength {
			maxLength = *field.MaxLength
		}
		if field.MinLength != nil && len([]rune(v)) < *field.MinLength || len([]rune(v)) > maxLength {
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
		switch value.(type) {
		case []string, []any:
		default:
			return badType
		}
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
	case "master", "user", "team", "ticket":
		ids, ok := stringsFromValue(value)
		if !ok || (!field.Multiple && len(ids) != 1) || field.Multiple && len(ids) > 10 {
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
	case "file":
		ids, ok := stringsFromValue(value)
		if !ok || len(ids) > 100 {
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

// pf14RoundStoredNumeric applies kickflow's 14-significant-digit persistence
// precision while keeping the JSON representation type used by the request.
func pf14RoundStoredNumeric(fieldType string, value any) any {
	if fieldType != "number" && fieldType != "integer" {
		return value
	}
	ok, n := pf14Float(value)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return value
	}
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(n, 'g', 14, 64), 64)
	if err != nil {
		return value
	}
	switch v := value.(type) {
	case string:
		if exact, err := pf14RoundDecimalString(v); err == nil {
			return exact
		}
		return strconv.FormatFloat(rounded, 'f', -1, 64)
	default:
		return rounded
	}
}

func pf14RoundDecimalString(value string) (string, error) {
	if len(value) > 4096 {
		return value, fmt.Errorf("decimal input is too large")
	}
	if marker := strings.IndexAny(value, "eE"); marker >= 0 {
		exponent, err := strconv.Atoi(value[marker+1:])
		if err != nil {
			return value, fmt.Errorf("invalid decimal exponent")
		}
		if exponent < -308 || exponent > 308 {
			return value, nil
		}
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok || r.Sign() == 0 {
		return value, fmt.Errorf("invalid decimal")
	}
	negative := r.Sign() < 0
	abs := new(big.Rat).Abs(r)
	// Bound the decimal order before constructing powers of ten. ParseFloat
	// underflows valid decimal strings such as 1e-400 to zero; converting
	// log10(0) to int can overflow and make big.Int.Exp allocate indefinitely.
	if r.Num().BitLen() > 4096 || r.Denom().BitLen() > 4096 {
		return value, fmt.Errorf("decimal magnitude is too large")
	}
	exponent := decimalOrder(abs)
	if exponent < -308 || exponent > 308 {
		return value, nil
	}
	comparePower := func(power int) int {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(power))), nil)
		bound := new(big.Rat).SetInt(p)
		if power < 0 {
			bound.Inv(bound)
		}
		return abs.Cmp(bound)
	}
	for comparePower(exponent) < 0 {
		exponent--
	}
	for comparePower(exponent+1) >= 0 {
		exponent++
	}
	scale := exponent - 13
	scaled := new(big.Rat).Set(abs)
	if scale >= 0 {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
		scaled.Quo(scaled, new(big.Rat).SetInt(divisor))
	} else {
		multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil)
		scaled.Mul(scaled, new(big.Rat).SetInt(multiplier))
	}
	quotient := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	remainder := new(big.Int).Rem(scaled.Num(), scaled.Denom())
	if new(big.Int).Lsh(remainder, 1).Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if negative {
		quotient.Neg(quotient)
	}
	result := new(big.Rat).SetInt(quotient)
	decimals := 0
	if scale >= 0 {
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
		result.Mul(result, new(big.Rat).SetInt(factor))
	} else {
		decimals = -scale
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		result.Quo(result, new(big.Rat).SetInt(divisor))
	}
	text := result.FloatString(decimals)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text, nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func decimalOrder(value *big.Rat) int {
	// Decimal input is at most a few kilobytes in API requests. Compare against
	// bounded powers to find its base-10 order without floating underflow.
	order := len(value.Num().String()) - len(value.Denom().String())
	if order >= 0 {
		bound := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(order)), nil)
		if value.Cmp(new(big.Rat).SetInt(bound)) < 0 {
			order--
		}
		return order
	}
	bound := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-order)), nil)
	if value.Cmp(new(big.Rat).SetFrac(big.NewInt(1), bound)) < 0 {
		order--
	}
	return order
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
		if len(field.AllowedTicketStatus) > 0 && !pf14cOptionAllowed(field.AllowedTicketStatus, "all") && !pf14cOptionAllowed(field.AllowedTicketStatus, status) {
			return false, nil
		}
		if len(field.SelectableTicketWorkflowCodes) == 0 {
			return true, nil
		}
		workflowID, exists, err := store.EngTicketWorkflowID(ctx, q, id)
		if err != nil || !exists {
			return exists, err
		}
		workflow, err := store.GetWorkflow(ctx, q, workflowID)
		if err != nil {
			return false, err
		}
		return pf14cOptionAllowed(field.SelectableTicketWorkflowCodes, workflow.Code), nil
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
func pf14ValidateApprovalInputs(ctx context.Context, q store.Querier, ticket *model.Ticket, def model.WorkflowDef, rows []model.TicketInput, slips []model.TicketSlipItem, activeRouteStepCodes map[string]bool) error {
	inputs := pf14InputsFromRows(def, rows)
	values := pf14InputValues(def, inputs)
	for _, section := range def.SectionList {
		visible, err := pf14SectionVisibleWithReferences(ctx, q, ticket, section, values)
		if err != nil {
			return err
		}
		if section.SectionType != model.SectionTypeForm || !visible {
			continue
		}
		for _, field := range section.FormFields {
			if field.Approver && pf14ApproverInputRequired(field.Required, field.ApproverEditRestriction, activeRouteStepCodes) && pf14Empty(values[field.Code]) {
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
			if !field.Approver || !pf14ApproverInputRequired(field.Required, field.ApproverEditRestriction, activeRouteStepCodes) || hidden || field.FieldType == "calculation" {
				continue
			}
			if cell, _ := slipCell(row, field); pf14Empty(cell) {
				return kferr.InputRequiredBeforeApprove()
			}
		}
	}
	return nil
}

func pf14ApproverInputRequired(required bool, restriction *model.ApproverEditRestriction, activeCodes map[string]bool) bool {
	if restriction == nil || len(restriction.RouteStepCodes) == 0 {
		return required
	}
	for _, code := range restriction.RouteStepCodes {
		if activeCodes[code] {
			return true
		}
	}
	return false
}
