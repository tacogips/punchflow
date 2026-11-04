package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"punchflow/internal/formula"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// calcDef is the part of a calculation field the evaluator needs.
type calcDef struct {
	id, code    string
	sectionType string
	formula     *string
	digits      *int
}

type slipRowRef struct {
	row     *model.TicketSlipItem
	section model.WorkflowSection
	state   map[string]int
}

// calcRun evaluates every calculation field of one normalized form. Formulas
// may reference form fields by code, detail columns as "section.field" (or the
// bare detail field code), other calculation fields, and, inside detail rows,
// the other cells of the same row.
type calcRun struct {
	n         *normalizer
	formVals  map[string]any
	formCalc  map[string]calcDef
	formState map[string]int
	rows      []*slipRowRef
	parsed    map[string]*formula.Expr
	badParse  map[string]bool
	changed   bool
	hidden    map[string]bool
	metadata  map[string]any
	globals   map[string]any
}

const (
	calcIdle = iota
	calcBusy
	calcDone
)

// calculate fills in calculation values for form fields and detail rows and
// returns the calculated title (when the workflow computes it) and whether any
// stored detail cell changed. Unsupported formulas are reported as validation
// errors; runtime failures such as division by zero or non-numeric operands
// leave the value null.
func (n *normalizer) calculate(inputs []model.TicketInput, slips []model.TicketSlipItem) (*string, bool) {
	r := &calcRun{n: n, formVals: map[string]any{}, formCalc: map[string]calcDef{}, formState: map[string]int{}, parsed: map[string]*formula.Expr{}, badParse: map[string]bool{}, hidden: map[string]bool{}, metadata: map[string]any{}}
	globals, err := store.CalculationVariables(n.ctx, n.q)
	if err != nil {
		n.fail("base", "could not load global calculation variables")
		globals = map[string]any{}
	}
	r.globals = globals
	if n.opts.AuthorID != "" {
		if user, err := store.GetUser(n.ctx, n.q, n.opts.AuthorID); err == nil {
			if value, err := calcUserJSON(n.ctx, n.q, user); err == nil {
				r.metadata["author"] = value
			} else {
				n.fail("base", "could not load public author custom fields")
			}
		}
	}
	if n.opts.ProxyClientID != nil {
		if user, err := store.GetUser(n.ctx, n.q, *n.opts.ProxyClientID); err == nil {
			if value, err := calcUserJSON(n.ctx, n.q, user); err == nil {
				r.metadata["proxyClientUser"] = value
			} else {
				n.fail("base", "could not load public proxy-client custom fields")
			}
		}
	} else {
		r.metadata["proxyClientUser"] = "null"
	}
	if len(n.opts.AuthorTeamJSON) != 0 {
		var team map[string]any
		if json.Unmarshal(n.opts.AuthorTeamJSON, &team) == nil {
			r.metadata["authorTeam"] = string(n.opts.AuthorTeamJSON)
		}
	}
	visibilityFormVals := map[string]any{}
	for i := range inputs {
		if f, ok := n.idx.fieldByID[inputs[i].FormFieldID]; ok {
			// Calculations run with the editing actor's view. Restricted values
			// remain stored on the ticket but are absent from this environment.
			if n.opts.Blocked[n.idx.fieldSection[f.ID]] {
				continue
			}
			if inputs[i].ReferenceSnapshot != nil {
				r.formVals[f.Code] = calcFormulaReferenceValue(f.FieldType, inputs[i].ReferenceSnapshot)
			} else {
				r.formVals[f.Code] = calcTypedValue(f.FieldType, n.calcReferenceValue(f.FieldType, inputs[i].RefIDs, inputs[i].Value))
			}
			if f.FieldType == "master" || f.FieldType == "user" || f.FieldType == "team" || f.FieldType == "ticket" {
				// Formula references are serialized as JSON records for formula
				// evaluation. Conditions compare selected IDs with resolved resource
				// thresholds, so retain a separate raw value for visibility checks.
				visibilityValue := inputs[i].Value
				if len(inputs[i].RefIDs) == 1 {
					visibilityValue = inputs[i].RefIDs[0]
				} else if len(inputs[i].RefIDs) > 1 {
					visibilityValue = inputs[i].RefIDs
				}
				visibilityFormVals[f.Code] = visibilityValue
			} else {
				visibilityFormVals[f.Code] = r.formVals[f.Code]
			}
		}
	}
	visibilityValues := make(map[string]any, len(visibilityFormVals)*2)
	for code, value := range visibilityFormVals {
		visibilityValues[code] = value
		if field, ok := n.idx.fieldByCode[code]; ok {
			visibilityValues[field.ID] = value
		}
	}
	for _, section := range n.def.SectionList {
		if section.SectionType != model.SectionTypeForm || n.opts.Blocked[section.ID] {
			continue
		}
		visible, visibilityErr := n.sectionVisible(section, visibilityValues)
		if visibilityErr != nil {
			n.fail(section.Code, "could not evaluate section visibility")
			continue
		}
		if !visible {
			r.hidden[section.ID] = true
		}
	}
	for _, section := range n.def.SectionList {
		for _, f := range section.FormFields {
			if f.FieldType == "calculation" && !n.opts.Blocked[section.ID] && !r.hidden[section.ID] && n.calculationEditable(f.Author, f.Approver) {
				r.formCalc[f.Code] = calcDef{id: f.ID, code: f.Code, sectionType: section.SectionType, formula: f.Formula, digits: f.DecimalDigit}
			}
		}
	}
	for i := range slips {
		if section, ok := n.idx.slipByID[slips[i].SlipSectionID]; ok {
			r.rows = append(r.rows, &slipRowRef{row: &slips[i], section: section, state: map[string]int{}})
		}
	}
	for _, sr := range r.rows {
		if n.opts.Blocked[sr.section.ID] {
			continue
		}
		for _, f := range sr.section.SlipFields {
			if f.FieldType == "calculation" && n.calculationEditable(f.Author, f.Approver) {
				r.slipValue(sr, f)
			}
		}
	}
	for _, section := range n.def.SectionList {
		for _, f := range section.FormFields {
			if cd, ok := r.formCalc[f.Code]; ok {
				r.formValue(cd)
			}
		}
	}
	for i := range inputs {
		if f, ok := n.idx.fieldByID[inputs[i].FormFieldID]; ok {
			if _, isCalc := r.formCalc[f.Code]; isCalc {
				inputs[i].Value = r.formVals[f.Code]
			}
		}
	}
	return r.title(), r.changed
}

func (n *normalizer) calculationEditable(author, approver bool) bool {
	if n.opts.Approver {
		return approver
	}
	return author
}

func calcTypedValue(fieldType string, value any) any {
	if fieldType != "number" && fieldType != "integer" {
		return value
	}
	if ok, number := pf14Float(value); ok {
		return number
	}
	return value
}

func (n *normalizer) calcReferenceValue(fieldType string, ids []string, value any) any {
	if len(ids) == 0 {
		return value
	}
	records := make([]any, 0, len(ids))
	for _, id := range ids {
		var record any
		switch fieldType {
		case "master":
			item, err := store.GetGeneralMasterItem(n.ctx, n.q, id)
			if err != nil {
				continue
			}
			record = map[string]any{"id": item.ID, "code": item.Code, "name": item.Name, "description": item.Description}
			fields, err := store.GetGeneralMasterFields(n.ctx, n.q, item.GeneralMasterID)
			if err != nil {
				continue
			}
			for _, field := range fields {
				if value, ok := item.Inputs[field.ID]; ok {
					record.(map[string]any)[field.Code] = value
				}
			}
		case "user":
			user, err := store.GetUser(n.ctx, n.q, id)
			if err != nil {
				continue
			}
			encoded, err := calcUserJSON(n.ctx, n.q, user)
			if err != nil {
				continue
			}
			record = json.RawMessage(encoded)
		case "team":
			team, err := store.GetTeam(n.ctx, n.q, id)
			if err != nil {
				continue
			}
			fullName, err := store.TeamFullName(n.ctx, n.q, id)
			if err != nil {
				continue
			}
			record = map[string]any{"id": team.ID, "name": team.Name, "fullName": fullName, "code": team.Code}
		default:
			return value
		}
		records = append(records, record)
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return value
	}
	return string(raw)
}

func calcUserJSON(ctx context.Context, q store.Querier, user model.User) (string, error) {
	fullName := strings.TrimSpace(user.LastName + " " + user.FirstName)
	customFields, err := pf14PublicUserCustomFields(ctx, q, user.CustomFields)
	if err != nil {
		return "", err
	}
	value := map[string]any{"id": user.ID, "firstName": user.FirstName, "lastName": user.LastName, "fullName": fullName, "email": user.Email, "code": user.Code, "customFields": customFields}
	if user.EmployeeID != nil {
		value["employeeId"] = *user.EmployeeID
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode formula user metadata: %w", err)
	}
	return string(raw), nil
}

func (r *calcRun) title() *string {
	d := r.n.def
	if d.TitleInputMode != "calculate" || d.TitleFormula == nil || strings.TrimSpace(*d.TitleFormula) == "" {
		return nil
	}
	value, ok := r.evalFormula("title", *d.TitleFormula, formScope{r}, nil)
	if !ok || value == nil {
		return nil
	}
	text := formula.FormatValue(value)
	return &text
}

func (r *calcRun) parse(code, src string) *formula.Expr {
	if expr, ok := r.parsed[src]; ok {
		return expr
	}
	if r.badParse[src] {
		return nil
	}
	expr, err := formula.Parse(src)
	if err != nil {
		r.badParse[src] = true
		r.n.fail(code, err.Error())
		return nil
	}
	r.parsed[src] = expr
	return expr
}

// evalFormula returns (value, true) on success. It reports unsupported or
// unresolvable formulas as validation errors under code and returns false;
// runtime failures return (nil, true).
func (r *calcRun) evalFormula(code, src string, env formula.Env, row *slipRowRef) (any, bool) {
	expr := r.parse(code, src)
	if expr == nil {
		return nil, false
	}
	for _, ref := range expr.Refs() {
		if !r.known(ref, row) {
			r.n.fail(code, fmt.Sprintf("has a formula referencing unknown field %q", ref))
			return nil, false
		}
	}
	for _, ref := range expr.SumRefs() {
		_, field, ok := r.n.slipColumn(ref)
		source, sourceOK := r.formCalc[code]
		if row != nil || !sourceOK || source.sectionType != model.SectionTypeForm || !ok || !field.ShowTotal {
			r.n.fail(code, fmt.Sprintf("has an invalid detail sum reference %q", ref))
			return nil, false
		}
	}
	value, err := expr.Eval(env)
	var evalErr *formula.EvalError
	if errors.As(err, &evalErr) {
		return nil, true
	}
	if err != nil {
		r.n.fail(code, err.Error())
		return nil, false
	}
	return value, true
}

func (r *calcRun) known(name string, row *slipRowRef) bool {
	if _, ok := r.metadata[name]; ok {
		return true
	}
	if _, ok := r.globals[name]; ok {
		return true
	}
	if row != nil {
		if _, ok := rowField(row.section, name); ok {
			return true
		}
	}
	if _, ok := r.n.idx.fieldByCode[name]; ok {
		return true
	}
	_, _, ok := r.n.slipColumn(name)
	return ok
}

// rowField resolves a detail field of the section by code or "section.code".
func rowField(section model.WorkflowSection, name string) (model.SlipField, bool) {
	code := strings.TrimPrefix(name, section.Code+".")
	for _, f := range section.SlipFields {
		if f.Code == code {
			return f, true
		}
	}
	return model.SlipField{}, false
}

// slipColumn resolves "section.field" or a bare detail field code to a detail
// column. A bare code matches the first detail section defining it.
func (n *normalizer) slipColumn(name string) (model.WorkflowSection, model.SlipField, bool) {
	for _, section := range n.idx.slipOrder {
		if strings.Contains(name, ".") && !strings.HasPrefix(name, section.Code+".") {
			continue
		}
		if f, ok := rowField(section, name); ok {
			return section, f, true
		}
	}
	return model.WorkflowSection{}, model.SlipField{}, false
}

func roundTo(x float64, digits *int) any {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	if digits != nil && *digits >= 0 {
		p := math.Pow(10, float64(*digits))
		x = math.Round(x*p) / p
	}
	return x
}

func calcResult(value any, digits *int) any {
	switch v := value.(type) {
	case float64:
		return roundTo(v, digits)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return value
}

func (r *calcRun) formValue(cd calcDef) any {
	switch r.formState[cd.code] {
	case calcDone:
		return r.formVals[cd.code]
	case calcBusy:
		r.n.fail(cd.code, "has a circular formula")
		return nil
	}
	r.formState[cd.code] = calcBusy
	var result any
	if cd.formula != nil && strings.TrimSpace(*cd.formula) != "" {
		if value, ok := r.evalFormula(cd.code, *cd.formula, formScope{r}, nil); ok {
			result = calcResult(value, cd.digits)
		}
	}
	r.formVals[cd.code] = result
	r.formState[cd.code] = calcDone
	return result
}

func (r *calcRun) slipValue(sr *slipRowRef, f model.SlipField) any {
	if !r.n.calculationEditable(f.Author, f.Approver) {
		v, _ := slipCell(*sr.row, f)
		return v
	}
	switch sr.state[f.ID] {
	case calcDone:
		v, _ := slipCell(*sr.row, f)
		return v
	case calcBusy:
		r.n.fail(f.Code, "has a circular formula")
		return nil
	}
	sr.state[f.ID] = calcBusy
	var result any
	if f.Formula != nil && strings.TrimSpace(*f.Formula) != "" {
		if value, ok := r.evalFormula(f.Code, *f.Formula, rowScope{r, sr}, sr); ok {
			result = calcResult(value, f.DecimalDigit)
		}
	}
	old, had := sr.row.Inputs[f.ID]
	if !reflect.DeepEqual(old, result) && (had || result != nil) {
		sr.row.Inputs[f.ID] = result
		r.changed = true
	}
	sr.state[f.ID] = calcDone
	return result
}

func (r *calcRun) cell(sr *slipRowRef, f model.SlipField) any {
	if f.FieldType == "calculation" {
		return r.slipValue(sr, f)
	}
	v, _ := slipCell(*sr.row, f)
	if snapshot, ok := sr.row.ReferenceSnapshots[f.ID]; ok {
		return calcFormulaReferenceValue(f.FieldType, snapshot)
	}
	return calcTypedValue(f.FieldType, r.n.calcReferenceValue(f.FieldType, pf14StringsOf(v), v))
}

func calcJSONValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return string(raw)
}

func calcFormulaReferenceValue(fieldType string, snapshot any) any {
	if raw, err := json.Marshal(snapshot); err == nil {
		var cloned any
		if json.Unmarshal(raw, &cloned) == nil {
			snapshot = cloned
		}
	}
	if fieldType == "master" {
		if records, ok := snapshot.([]any); ok {
			for _, raw := range records {
				record, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				inputs, ok := record["inputs"].([]any)
				if !ok {
					continue
				}
				for _, rawInput := range inputs {
					input, ok := rawInput.(map[string]any)
					if !ok {
						continue
					}
					field, ok := input["field"].(map[string]any)
					if ok {
						if code, codeOK := field["code"].(string); codeOK {
							record[code] = input["value"]
						}
					}
				}
			}
		}
	}
	return calcJSONValue(snapshot)
}

type formScope struct{ r *calcRun }

func (s formScope) Lookup(name string) (any, bool) {
	r := s.r
	// Kickflow resolves workflow form values before metadata, then globals.
	if f, ok := r.n.idx.fieldByCode[name]; ok {
		if r.hidden[r.n.idx.fieldSection[f.ID]] {
			return nil, false
		}
		if cd, isCalc := r.formCalc[f.Code]; isCalc {
			return r.formValue(cd), true
		}
		return r.formVals[f.Code], true
	}
	section, field, ok := r.n.slipColumn(name)
	if ok {
		column := []any{}
		for _, sr := range r.rows {
			if sr.section.ID == section.ID {
				column = append(column, r.cell(sr, field))
			}
		}
		return column, true
	}
	if value, ok := r.metadata[name]; ok {
		return value, true
	}
	if value, ok := r.globals[name]; ok {
		return value, true
	}
	return nil, false
}

type rowScope struct {
	r  *calcRun
	sr *slipRowRef
}

func (s rowScope) Lookup(name string) (any, bool) {
	if f, ok := rowField(s.sr.section, name); ok {
		return s.r.cell(s.sr, f), true
	}
	return formScope{s.r}.Lookup(name)
}
