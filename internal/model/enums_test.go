package model

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnumListsMatchOpenAPI(t *testing.T) {
	data, err := os.ReadFile("../../design-docs/references/kickflow-openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name    string
		values  []string
		schema  string
		path    []string
		exclude []string
	}{
		{"TicketStatuses", TicketStatuses, "TicketSummary", []string{"status"}, []string{"permanently_deleted"}},
		{"StepStatuses", StepStatuses, "TicketStep", []string{"status"}, []string{"raised"}},
		{"ActionTypes", ActionTypes, "RouteStep", []string{"actionType"}, nil},
		{"StepTypes", StepTypes, "RouteStep", []string{"stepType"}, nil},
		{"FallbackTypes", FallbackTypes, "RouteStep", []string{"fallbackType"}, nil},
		{"ManagerSelectionTypes", ManagerSelectionTypes, "RouteStep", []string{"managerSelectionType"}, nil},
		{"GradeSymbols", GradeSymbols, "RouteStepTarget", []string{"gradeSymbol"}, nil},
		{"StepConditionTypes", StepConditionTypes, "RouteStepCondition", []string{"conditionType"}, nil},
		{"StepCombinationTypes", StepCombinationTypes, "RouteStepCondition", []string{"combinationType"}, nil},
		{"StepConditionFieldKeys", StepConditionFieldKeys, "RouteStepConditionField", []string{"fieldKey"}, []string{"other_variable"}},
		{"RouteConditionTypes", RouteConditionTypes, "WorkflowRouteCondition", []string{"conditionType"}, nil},
		{"RouteCombinationTypes", RouteCombinationTypes, "WorkflowRouteCondition", []string{"combinationType"}, []string{"custom"}},
		{"SectionTypes", SectionTypes, "SectionListItem", []string{"sectionType"}, nil},
		{"RouteStatuses", RouteStatuses, "Route", []string{"status"}, nil},
		{"WorkflowStatuses", WorkflowStatuses, "Workflow", []string{"status"}, nil},
		{"UserStatuses", UserStatuses, "User", []string{"status"}, nil},
		{"UserTypes", UserTypes, "User", []string{"userType"}, nil},
		{"Permissions", Permissions, "RoleDetail", []string{"permissionList", "permission"}, nil},
		{"MasterFieldTypes", MasterFieldTypes, "GeneralMasterField", []string{"fieldType"}, nil},
		{"MasterSortKeys", MasterSortKeys, "GeneralMaster", []string{"defaultSortBy"}, nil},
		{"FormFieldSizes", FormFieldSizes, "FormField", []string{"size"}, nil},
		{"ForcedPublicTypes", ForcedPublicTypes, "TicketSummary", []string{"forcedPublicType"}, nil},
	}
	if len(rows) != 22 {
		t.Fatalf("enum rows=%d; want 22", len(rows))
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			want := pf02Enum(document, row.schema, row.path...)
			want = pf02Difference(want, row.exclude)
			gotSet, wantSet := append([]string(nil), row.values...), append([]string(nil), want...)
			sort.Strings(gotSet)
			sort.Strings(wantSet)
			if !reflect.DeepEqual(gotSet, wantSet) {
				t.Errorf("%s missing=%v extra=%v", row.name, pf02Difference(wantSet, gotSet), pf02Difference(gotSet, wantSet))
			}
		})
	}
}

func TestWebhookEventTypesMatchReference(t *testing.T) {
	want := []string{"ping", "ticket_created", "ticket_updated", "ticket_opened", "ticket_approved", "ticket_confirmed", "ticket_rejected", "ticket_denied", "ticket_completed", "ticket_withdrawn", "ticket_archived", "comment_created", "comment_updated"}
	if !reflect.DeepEqual(WebhookEventTypes, want) {
		t.Fatalf("WebhookEventTypes=%v; want %v", WebhookEventTypes, want)
	}
}

func TestEnumListsHaveNoDuplicates(t *testing.T) {
	lists := map[string][]string{
		"TicketStatuses": TicketStatuses, "StepStatuses": StepStatuses, "ActionTypes": ActionTypes,
		"StepTypes": StepTypes, "FallbackTypes": FallbackTypes, "ManagerSelectionTypes": ManagerSelectionTypes,
		"GradeSymbols": GradeSymbols, "StepConditionTypes": StepConditionTypes, "StepCombinationTypes": StepCombinationTypes,
		"StepConditionFieldKeys": StepConditionFieldKeys, "RouteConditionTypes": RouteConditionTypes,
		"RouteCombinationTypes": RouteCombinationTypes, "SectionTypes": SectionTypes, "RouteStatuses": RouteStatuses,
		"WorkflowStatuses": WorkflowStatuses, "UserStatuses": UserStatuses, "UserTypes": UserTypes,
		"Permissions": Permissions, "MasterFieldTypes": MasterFieldTypes, "MasterSortKeys": MasterSortKeys,
		"FormFieldSizes": FormFieldSizes, "ForcedPublicTypes": ForcedPublicTypes,
		"WebhookEventTypes": WebhookEventTypes,
	}
	for name, values := range lists {
		t.Run(name, func(t *testing.T) {
			seen := make(map[string]bool, len(values))
			for _, value := range values {
				if seen[value] {
					t.Errorf("duplicate value %q", value)
				}
				seen[value] = true
			}
		})
	}
}

func pf02Enum(document map[string]any, schema string, path ...string) []string {
	components := pf02Map(document["components"])
	schemas := pf02Map(components["schemas"])
	node := schemas[schema]
	for _, property := range path {
		node = pf02FindProperty(node, property)
		if node == nil {
			panic(fmt.Sprintf("OpenAPI property %s.%s not found", schema, property))
		}
	}
	values := make([]string, 0)
	pf02CollectEnums(node, &values)
	return values
}

func pf02FindProperty(node any, name string) any {
	object := pf02Map(node)
	if properties := pf02Map(object["properties"]); properties[name] != nil {
		return properties[name]
	}
	if items := object["items"]; items != nil {
		if found := pf02FindProperty(items, name); found != nil {
			return found
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if members, ok := object[key].([]any); ok {
			for _, member := range members {
				if found := pf02FindProperty(member, name); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

func pf02CollectEnums(node any, values *[]string) {
	object := pf02Map(node)
	if items := object["items"]; items != nil {
		pf02CollectEnums(items, values)
	}
	if enums, ok := object["enum"].([]any); ok {
		for _, value := range enums {
			if text, ok := value.(string); ok {
				*values = append(*values, text)
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if members, ok := object[key].([]any); ok {
			for _, member := range members {
				pf02CollectEnums(member, values)
			}
		}
	}
}

func pf02Map(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return nil
}

func pf02Difference(left, right []string) []string {
	rightSet := make(map[string]bool, len(right))
	for _, value := range right {
		rightSet[value] = true
	}
	missing := make([]string, 0)
	for _, value := range left {
		if !rightSet[value] {
			missing = append(missing, value)
		}
	}
	return missing
}
