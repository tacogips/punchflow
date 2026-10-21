package engine

import (
	"reflect"
	"testing"

	"punchflow/internal/model"
)

func TestConditionOperators(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		symbol   string
		expected any
		want     bool
	}{
		{"equal number", 60000, "equal", 60000, true},
		{"equal string", "yes", "equal", "yes", true},
		{"not equal", "yes", "not_equal", "no", true},
		{"greater than number", 60000, "greater_than", 50000, true},
		{"greater than numeric string", "60000", "greater_than", 50000, true},
		{"greater than or equal", 5, "greater_than_or_equal", 5, true},
		{"less than", 4, "less_than", 5, true},
		{"less than or equal", 5, "less_than_or_equal", 5, true},
		{"include checkbox", []any{"yes", "no"}, "include", "yes", true},
		{"include scalar", "yes", "include", "yes", true},
		{"exclude checkbox", []any{"yes"}, "exclude", "no", true},
		{"exclude matching checkbox", []any{"yes"}, "exclude", "yes", false},
		{"is empty nil", nil, "is_empty", nil, true},
		{"is empty string", "", "is_empty", nil, true},
		{"is empty array", []any{}, "is_empty", nil, true},
		{"is not empty", "x", "is_not_empty", nil, true},
		{"descendants equal", "team-a", "descendants_or_equal", "team-a", true},
		{"not descendants equal", "team-a", "not_descendants_or_equal", "team-b", true},
		{"unknown", "x", "unsupported", "x", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pf14Compare(test.actual, test.symbol, test.expected); got != test.want {
				t.Fatalf("pf14Compare(%#v, %q, %#v)=%v, want %v", test.actual, test.symbol, test.expected, got, test.want)
			}
		})
	}
	fields := []model.ConditionField{{FieldKey: "a", Symbol: "equal", Value: true}, {FieldKey: "b", Symbol: "equal", Value: true}}
	if !pf14ConditionMatch(fields, model.StepCombinationAll, map[string]any{"a": true, "b": true}) {
		t.Fatal("all combination should match both values")
	}
	if !pf14ConditionMatch(fields, model.StepCombinationAny, map[string]any{"a": true, "b": false}) {
		t.Fatal("any combination should match one value")
	}
}

func TestInputValuesResolveIDOnlyInputs(t *testing.T) {
	def := model.WorkflowDef{SectionList: []model.WorkflowSection{{
		FormFields: []model.FormField{{ID: "field-amount", Code: "amount"}},
	}}}

	idOnly := pf14Input("", "field-amount", 42)
	got := pf14InputValues(def, []FieldInput{idOnly})
	if got["field-amount"] != float64(42) || got["amount"] != float64(42) {
		t.Fatalf("ID-only input values=%#v, want field ID and code mapped to 42", got)
	}

	codeOnly := pf14Input("amount", "", 42)
	got = pf14InputValues(def, []FieldInput{codeOnly})
	wantCodeOnly := map[string]any{"": float64(42), "amount": float64(42)}
	if !reflect.DeepEqual(got, wantCodeOnly) {
		t.Fatalf("code-only input values=%#v, want unchanged map %#v", got, wantCodeOnly)
	}

	explicitCode := pf14Input("amount", "", 7)
	got = pf14InputValues(def, []FieldInput{idOnly, explicitCode})
	if got["amount"] != float64(7) {
		t.Fatalf("explicit code value=%#v, want 7 to take precedence over ID-only value", got["amount"])
	}
	got = pf14InputValues(def, []FieldInput{explicitCode, idOnly})
	if got["amount"] != float64(7) {
		t.Fatalf("ID-only value overwrote explicit code value: %#v", got["amount"])
	}
}
