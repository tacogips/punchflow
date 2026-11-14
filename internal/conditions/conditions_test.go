package conditions

import (
	"testing"

	"punchflow/internal/model"
)

func TestINFSEC06MatchVariableAndNumericComparisons(t *testing.T) {
	tests := []struct {
		name   string
		field  model.ConditionField
		values map[string]any
		want   bool
	}{
		{
			name:   "default fixture reason variable",
			field:  model.ConditionField{FieldKey: "text_variable", Variable: "reason", Symbol: "equal", Value: "other"},
			values: map[string]any{"reason": "other"}, want: true,
		},
		{
			name:   "normalized variable and numeric comparison",
			field:  model.ConditionField{FieldKey: "number_variable", Variable: "$amount", Symbol: "greater_than_or_equal", Value: 1000},
			values: map[string]any{"amount": float64(1000)}, want: true,
		},
		{
			name:   "numeric comparison rejects lower value",
			field:  model.ConditionField{FieldKey: "number_variable", Variable: "${amount}", Symbol: "greater_than", Value: 1000},
			values: map[string]any{"amount": float64(999)}, want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Match([]model.ConditionField{test.field}, model.StepCombinationAll, test.values); got != test.want {
				t.Fatalf("Match()=%v, want %v", got, test.want)
			}
		})
	}
}
