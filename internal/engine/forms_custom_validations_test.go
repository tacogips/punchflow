package engine

import (
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

func TestCustomValidationsOnFormFields(t *testing.T) {
	for _, tc := range []struct {
		name        string
		combination string
		amount      string
		wantError   bool
	}{
		{name: "all matched", combination: "all", amount: "150", wantError: true},
		{name: "all partially matched", combination: "all", amount: "101"},
		{name: "any matched", combination: "any", amount: "101", wantError: true},
		{name: "any unmatched", combination: "any", amount: "100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pf14Setup(t)
			_, fields := pf14FormDefinition(t, e, "expense-claim")
			gt100, equal150 := "100", "150"
			pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
				for si := range def.SectionList {
					for fi := range def.SectionList[si].FormFields {
						field := &def.SectionList[si].FormFields[fi]
						if field.Code == "purpose" {
							field.CustomValidations = []model.FormCustomValidation{{
								ID: "a1000000-0000-4000-8000-000000000001", CombinationType: tc.combination, ErrorMessage: "Amount range matches", Fields: []model.FormCustomValidationField{
									{ID: "a1000000-0000-4000-8000-000000000002", FormFieldCode: "amount", Symbol: "greater_than", Value: &gt100},
									{ID: "a1000000-0000-4000-8000-000000000003", FormFieldCode: "amount", Symbol: "equal", Value: &equal150},
								},
							}}
						}
					}
				}
			})
			inputs := pf14CompleteInputs(fields, map[string]any{"amount": tc.amount, "purpose": "travel"})
			_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, nil)
			if tc.wantError {
				api, ok := kferr.As(err)
				if !ok || api.Code != "validation_failed" || len(api.Errors["purpose"]) == 0 || api.Errors["purpose"][0] != "Amount range matches" {
					t.Fatalf("create error=%v; want custom validation message", err)
				}
			} else if err != nil {
				t.Fatalf("create unexpectedly failed: %v", err)
			}
		})
	}
}

func TestCustomValidationsOnDetailFields(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	qtyThreshold, priceThreshold := "2", "10"
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		for si := range def.SectionList {
			section := &def.SectionList[si]
			if section.Code != "lines" {
				continue
			}
			for fi := range section.SlipFields {
				if section.SlipFields[fi].Code == "item" {
					section.SlipFields[fi].CustomValidations = []model.SlipCustomValidation{{
						ID: "a2000000-0000-4000-8000-000000000001", CombinationType: "all", ErrorMessage: "Detail total is restricted", Fields: []model.SlipCustomValidationField{
							{ID: "a2000000-0000-4000-8000-000000000002", SlipFieldCode: "qty", Symbol: "greater_than", Value: &qtyThreshold},
							{ID: "a2000000-0000-4000-8000-000000000003", SlipFieldCode: "unit-price", Symbol: "greater_than", Value: &priceThreshold},
						},
					}}
				}
			}
		}
	})
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": "30", "purpose": "travel"})
	_, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, []SlipItemInput{pf16Row("lines", map[string]any{"item": "pencil", "qty": "3", "unit-price": "11"})})
	api, ok := kferr.As(err)
	if !ok || api.Code != "validation_failed" || len(api.Errors["item"]) == 0 || api.Errors["item"][0] != "Detail total is restricted" {
		t.Fatalf("create error=%v; want custom detail validation message (workflow=%s)", err, workflow.ID)
	}
	_, err = pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, []SlipItemInput{pf16Row("lines", map[string]any{"item": "pencil", "qty": "1", "unit-price": "11"})})
	if err != nil {
		t.Fatalf("detail conditions not met, create error=%v", err)
	}
}

func TestCustomValidationEmptyConditionMatchesOmittedValue(t *testing.T) {
	field := model.FormCustomValidationField{FormFieldCode: "optional", Symbol: "is_empty"}
	if !pf12CustomValidationMatches("all", []model.FormCustomValidationField{field}, func(string) (any, bool) { return nil, false }) {
		t.Fatal("is_empty did not match an omitted form value")
	}
}
