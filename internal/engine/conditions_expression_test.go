package engine

import (
	"testing"

	"punchflow/internal/model"
)

func TestCustomRouteAndSectionConditionExpressions(t *testing.T) {
	fields := []model.ConditionField{
		{Variable: "a", Symbol: "equal", Value: "yes"},
		{Variable: "b", Symbol: "equal", Value: "yes"},
		{Variable: "c", Symbol: "equal", Value: "yes"},
	}
	values := map[string]any{"a": "no", "b": "yes", "c": "yes"}
	if !pf14SectionVisible(model.WorkflowSection{SectionType: model.SectionTypeForm, Conditional: true, CombinationType: "custom", CombinationExpression: "(A || B) && C", ConditionFields: fields}, values) {
		t.Fatal("conditional section should be visible when custom expression matches")
	}
	if pf14SectionVisible(model.WorkflowSection{SectionType: model.SectionTypeForm, Conditional: true, CombinationType: "custom", CombinationExpression: "A && C", ConditionFields: fields}, values) {
		t.Fatal("conditional section should be hidden when custom expression does not match")
	}
	workflow := model.WorkflowDef{
		RouteCode: "fallback",
		RouteConditions: []model.RouteCondition{
			{ConditionType: model.RouteConditionTypeField, CombinationType: "custom", CombinationExpression: "(A || B) && C", ConditionFields: fields, RouteCode: func() *string { value := "selected"; return &value }()},
			{ConditionType: model.RouteConditionTypeFieldOtherwise, CombinationType: "all", RouteCode: func() *string { value := "otherwise"; return &value }()},
		},
	}
	if got, err := pf14SelectRouteCode(workflow, values); err != nil || got != "selected" {
		t.Fatalf("selected route=%q err=%v, want selected", got, err)
	}
}
