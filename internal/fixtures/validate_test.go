package fixtures

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"punchflow/internal/model"
)

func TestValidateMinimal(t *testing.T) {
	s, err := Parse(pf06FSSub(t, "testdata/minimal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("Validate(minimal): %v", err)
	}
}

func TestValidateCustomRouteAndSectionExpressions(t *testing.T) {
	set, err := Parse(pf06FSSub(t, "testdata/minimal"))
	if err != nil {
		t.Fatal(err)
	}
	wf := &set.Workflows[0]
	conditionField := model.ConditionField{ID: "00000000-0000-4000-8000-000000000031", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}
	wf.RouteConditions = []model.RouteCondition{
		{ID: "00000000-0000-4000-8000-000000000032", ConditionType: model.RouteConditionTypeField, CombinationType: "custom", CombinationExpression: "(A || B) && C", RouteCode: strPtr("ROUTE"), ConditionFields: []model.ConditionField{conditionField, {ID: "00000000-0000-4000-8000-000000000033", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}, {ID: "00000000-0000-4000-8000-000000000034", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}}},
		{ID: "00000000-0000-4000-8000-000000000035", ConditionType: model.RouteConditionTypeAlways, RouteCode: strPtr("ROUTE")},
	}
	wf.SectionList = []model.WorkflowSection{{ID: "00000000-0000-4000-8000-000000000036", Title: "Conditional", SectionType: model.SectionTypeForm, Conditional: true, CombinationType: "custom", CombinationExpression: "A && B", ConditionFields: []model.ConditionField{{ID: "00000000-0000-4000-8000-000000000038", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}, {ID: "00000000-0000-4000-8000-000000000037", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}}}}
	if err := Validate(set); err != nil {
		t.Fatalf("custom conditions should validate: %v", err)
	}
}

func TestValidateRouteOtherVariableCondition(t *testing.T) {
	set, err := Parse(pf06FSSub(t, "testdata/minimal"))
	if err != nil {
		t.Fatal(err)
	}
	set.Routes[0].Steps[0].RouteStepCondition = &model.RouteStepCondition{
		ID:                       "00000000-0000-4000-8000-000000000041",
		ConditionType:            model.StepConditionTypeConditional,
		CombinationType:          model.StepCombinationAll,
		RouteStepConditionFields: []model.ConditionField{{ID: "00000000-0000-4000-8000-000000000042", FieldKey: "other_variable", Variable: "$external", Symbol: "is_empty"}},
	}
	if err := Validate(set); err != nil {
		t.Fatalf("other_variable condition should validate: %v", err)
	}
}

func strPtr(value string) *string { return &value }

func TestValidateInvalidCases(t *testing.T) {
	cases := []struct {
		name string
		file string
		path string
	}{
		{"unknown-key", FileTenant, "unexpected"},
		{"duplicate-id", FileGrades, "$[1].id"},
		{"bad-enum", FileUsers, "[0].status"},
		{"dangling-user-code", FileRoles, "[0].memberCodes[1]"},
		{"two-current-charts", FileOrganizationCharts, "$"},
		{"custom-combination", FileWorkflows, "[0].routeConditions[0].combinationExpression"},
		{"both-thresholds", FileRoutes, "[0].steps[0]"},
		{"route-no-else", FileWorkflows, "[0].routeConditions"},
		{"pat-without-user", FileTokens, "[0].userCode"},
		{"step-type-grade", FileRoutes, "[0].steps[0].stepType"},
		{"permission-audit-read", FileRoles, "[0].permissionList[0].permission"},
		{"event-ticket-submitted", FileWebhooks, "[0].eventTypes[0]"},
		{"user-type-member", FileUsers, "[0].userType"},
		{"route-status-active", FileRoutes, "[0].status"},
		{"bad-numbering-timing", FileWorkflows, "[0].ticketNumberingTiming"},
		{"section-viewer-unknown-user", FileWorkflows, "[0].sectionList[0].sectionViewers[0].userCode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set, err := Parse(pf06FSSub(t, "testdata/invalid/"+tc.name))
			var validationErr *ValidationError
			if err != nil {
				if !errors.As(err, &validationErr) {
					t.Fatalf("Parse error=%v; want ValidationError", err)
				}
			} else {
				err = Validate(set)
				if !errors.As(err, &validationErr) {
					t.Fatalf("Validate error=%v; want ValidationError", err)
				}
			}
			if len(validationErr.Problems) != 1 {
				t.Fatalf("got %d problems: %+v; want exactly one", len(validationErr.Problems), validationErr.Problems)
			}
			got := validationErr.Problems[0]
			if got.File != tc.file || got.Path != tc.path {
				t.Fatalf("problem=%+v; want file=%s path=%s", got, tc.file, tc.path)
			}
		})
	}
}

func pf06FSSub(t *testing.T, dir string) fs.FS {
	t.Helper()
	root := os.DirFS(".")
	sub, err := fs.Sub(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}
