package conditions

import (
	"testing"

	"punchflow/internal/model"
)

func TestMatchExpression(t *testing.T) {
	fields := []model.ConditionField{
		{Variable: "a", Symbol: "equal", Value: "yes"},
		{Variable: "b", Symbol: "equal", Value: "yes"},
		{Variable: "c", Symbol: "equal", Value: "yes"},
	}
	values := map[string]any{"a": "no", "b": "yes", "c": "yes"}
	if !MatchExpression(fields, "custom", "(A || B) && C", values) {
		t.Fatal("custom expression should match B and C")
	}
	if MatchExpression(fields, "custom", "A && C", values) {
		t.Fatal("custom expression matched false A")
	}
}

func TestValidateExpression(t *testing.T) {
	for _, test := range []struct {
		expression string
		count      int
	}{{"(A || B) && C", 3}, {"(A && B) || (C && D)", 4}} {
		if err := ValidateExpression(test.expression, test.count); err != nil {
			t.Errorf("ValidateExpression(%q): %v", test.expression, err)
		}
	}
	for _, expression := range []string{"", "A &&", "A || D", "A B", "(A || B"} {
		if err := ValidateExpression(expression, 3); err == nil {
			t.Errorf("ValidateExpression(%q) unexpectedly succeeded", expression)
		}
	}
	if err := ValidateExpression("A", 11); err == nil {
		t.Fatal("ValidateExpression accepted more than ten conditions")
	}
}
