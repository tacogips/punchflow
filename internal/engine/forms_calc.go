package engine

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"punchflow/internal/formula"
	"punchflow/internal/model"
)

// calcDef is the part of a calculation field the evaluator needs.
type calcDef struct {
	id, code string
	formula  *string
	digits   *int
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
	r := &calcRun{n: n, formVals: map[string]any{}, formCalc: map[string]calcDef{}, formState: map[string]int{}, parsed: map[string]*formula.Expr{}, badParse: map[string]bool{}}
	for i := range inputs {
		if f, ok := n.idx.fieldByID[inputs[i].FormFieldID]; ok {
			r.formVals[f.Code] = inputs[i].Value
		}
	}
	for _, section := range n.def.SectionList {
		for _, f := range section.FormFields {
			if f.FieldType == "calculation" && !n.opts.Blocked[section.ID] {
				r.formCalc[f.Code] = calcDef{id: f.ID, code: f.Code, formula: f.Formula, digits: f.DecimalDigit}
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
			if f.FieldType == "calculation" {
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
	return v
}

type formScope struct{ r *calcRun }

func (s formScope) Lookup(name string) (any, bool) {
	r := s.r
	if f, ok := r.n.idx.fieldByCode[name]; ok {
		if cd, isCalc := r.formCalc[f.Code]; isCalc {
			return r.formValue(cd), true
		}
		return r.formVals[f.Code], true
	}
	section, field, ok := r.n.slipColumn(name)
	if !ok {
		return nil, false
	}
	column := []any{}
	for _, sr := range r.rows {
		if sr.section.ID == section.ID {
			column = append(column, r.cell(sr, field))
		}
	}
	return column, true
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
