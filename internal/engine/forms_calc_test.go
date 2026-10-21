package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf16Formula(s string) *string { return &s }

// pf16WithCalcs adds calculation fields to the expense-claim workflow:
// form "total" = amount * 2, form "grand" = SUM of the detail subtotals + total,
// detail "subtotal" = qty * unit-price.
func pf16WithCalcs(t *testing.T, e *pf14TestEnv, mutate func(*model.WorkflowDef)) {
	t.Helper()
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		subtotal := pf16SlipField("subtotal", "calculation")
		subtotal.Formula = pf16Formula("qty * {unit-price}")
		pf16Section(def, "lines").SlipFields = append(pf16Section(def, "lines").SlipFields, subtotal)
		total := pf16FormField("total", "calculation")
		total.Formula = pf16Formula("amount * 2")
		grand := pf16FormField("grand", "calculation")
		grand.Formula = pf16Formula("SUM(lines.subtotal) + total")
		for i := range def.SectionList {
			if def.SectionList[i].SectionType == model.SectionTypeForm {
				def.SectionList[i].FormFields = append(def.SectionList[i].FormFields, total, grand)
				break
			}
		}
		if mutate != nil {
			mutate(def)
		}
	})
}

func pf16FormValue(t *testing.T, e *pf14TestEnv, id, code string) any {
	t.Helper()
	def := pf16Def(t, e, "expense-claim")
	for _, section := range def.SectionList {
		for _, f := range section.FormFields {
			if f.Code != code {
				continue
			}
			for _, row := range mustTicketInputs(t, e, id) {
				if row.FormFieldID == f.ID {
					return row.Value
				}
			}
		}
	}
	t.Fatalf("no stored input for %s", code)
	return nil
}

func TestCalculationFieldsAreEvaluated(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, nil)
	slips := []SlipItemInput{
		pf16Row("lines", map[string]any{"item": "pen", "qty": 2, "unit-price": 10.5}),
		pf16Row("lines", map[string]any{"item": "pad", "qty": "3", "unit-price": "4"}),
	}
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "trip"}), slips)
	if err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != 200.0 {
		t.Errorf("total=%v want 200", got)
	}
	// SUM over the detail column (2*10.5 + 3*4) + total.
	if got := pf16FormValue(t, e, id, "grand"); got != 233.0 {
		t.Errorf("grand=%v want 233", got)
	}
	def := pf16Def(t, e, "expense-claim")
	rows := pf16Slips(t, e, id)
	if len(rows) != 2 || pf16Cell(t, def, rows[0], "subtotal") != 21.0 || pf16Cell(t, def, rows[1], "subtotal") != 12.0 {
		t.Fatalf("detail subtotals wrong: %+v", rows)
	}

	// Recalculated when an input changes (partial form update of one field).
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("amount", "", "300")}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != 600.0 {
		t.Errorf("total after update=%v want 600", got)
	}
	if got := pf16FormValue(t, e, id, "grand"); got != 633.0 {
		t.Errorf("grand after update=%v want 633", got)
	}
	// ... and when detail rows change.
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 1})}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "grand"); got != 601.0 {
		t.Errorf("grand after detail change=%v want 601", got)
	}
	// A client-supplied value for a calculation field is ignored.
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("total", "", 1)}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16FormValue(t, e, id, "total"); got != 600.0 {
		t.Errorf("total after client override=%v want 600", got)
	}
}

func TestCalculatedValuesDriveRouteConditions(t *testing.T) {
	e := pf14Setup(t)
	pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
		for i := range def.RouteConditions {
			for j := range def.RouteConditions[i].ConditionFields {
				if def.RouteConditions[i].ConditionFields[j].Variable == "amount" {
					def.RouteConditions[i].ConditionFields[j].Variable = "total"
				}
			}
		}
	})
	// amount 60000 -> total 120000 >= 100000 selects the large-expense route.
	id, err := pf16Create(e, "sales-rep", model.TicketStatusInProgress, pf16Inputs(t, e, map[string]any{"amount": "60000", "purpose": "trip"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	routeID := e.ticket(id).RouteID
	if routeID == nil {
		t.Fatal("ticket has no route")
	}
	route, err := store.RouteDefByID(e.ctx, e.db.Q(), *routeID)
	if err != nil {
		t.Fatal(err)
	}
	if route.Code != "expense-large" {
		t.Fatalf("route=%s want expense-large (calculated total must be visible to route conditions)", route.Code)
	}
}

func TestUnsupportedAndFailingFormulas(t *testing.T) {
	t.Run("unsupported expression is a validation error", func(t *testing.T) {
		e := pf14Setup(t)
		pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
			for i := range def.SectionList {
				for j := range def.SectionList[i].FormFields {
					if def.SectionList[i].FormFields[j].Code == "total" {
						def.SectionList[i].FormFields[j].Formula = pf16Formula("json_lookup(amount, 1)")
					}
				}
			}
		})
		_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
		assertPF14FieldError(t, err, "validation_failed", "total")
	})
	t.Run("unknown reference is a validation error", func(t *testing.T) {
		e := pf14Setup(t)
		pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
			for i := range def.SectionList {
				for j := range def.SectionList[i].FormFields {
					if def.SectionList[i].FormFields[j].Code == "total" {
						def.SectionList[i].FormFields[j].Formula = pf16Formula("nonexistent * 2")
					}
				}
			}
		})
		_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
		assertPF14FieldError(t, err, "validation_failed", "total")
	})
	t.Run("circular references are rejected", func(t *testing.T) {
		e := pf14Setup(t)
		pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
			for i := range def.SectionList {
				for j := range def.SectionList[i].FormFields {
					if def.SectionList[i].FormFields[j].Code == "total" {
						def.SectionList[i].FormFields[j].Formula = pf16Formula("grand + 1")
					}
				}
			}
		})
		_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
		if err == nil {
			t.Fatal("circular formulas accepted")
		}
	})
	t.Run("runtime failure stores null", func(t *testing.T) {
		e := pf14Setup(t)
		pf16WithCalcs(t, e, func(def *model.WorkflowDef) {
			for i := range def.SectionList {
				for j := range def.SectionList[i].FormFields {
					if def.SectionList[i].FormFields[j].Code == "total" {
						def.SectionList[i].FormFields[j].Formula = pf16Formula("amount / 0")
					}
				}
			}
		})
		id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, pf16Base), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := pf16FormValue(t, e, id, "total"); got != nil {
			t.Fatalf("total=%v want null for division by zero", got)
		}
	})
}

func TestTitleFormulaIsCalculated(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		def.TitleInputMode = "calculate"
		def.TitleFormula = pf16Formula(`CONCAT("Expense: ", purpose, " (", ROUND(amount * 1.1, 0), ")")`)
	})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, pf16Inputs(t, e, map[string]any{"amount": "100", "purpose": "trip"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	title := e.ticket(id).Title
	if title == nil || *title != "Expense: trip (110)" {
		t.Fatalf("title=%v want %q", title, "Expense: trip (110)")
	}
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("purpose", "", "taxi")}}); err != nil {
		t.Fatal(err)
	}
	if title = e.ticket(id).Title; title == nil || *title != "Expense: taxi (110)" {
		t.Fatalf("title after update=%v", title)
	}
}
