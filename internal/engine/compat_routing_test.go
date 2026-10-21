package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// PF-10: dynamic assignee variables accept the documented "$field_code" syntax
// and their values are user/team codes (IDs keep working).
func TestDynamicAssigneeVariablesResolveCodes(t *testing.T) {
	e := pf14Setup(t)
	resolve := func(step model.RouteStep, values map[string]any) []string {
		t.Helper()
		ticket := model.Ticket{AuthorID: compatStr(e.user("sales-rep")), AuthorTeamID: compatStr(e.team("sales"))}
		var ids []string
		e.apply(func(tx *store.Tx) error {
			var err error
			ids, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, step, values, nil)
			return err
		})
		return ids
	}
	userStep := model.RouteStep{StepType: model.StepTypeDynamicUser, Targets: []model.RouteTarget{{Variable: "$reviewer"}}}
	cases := []struct {
		name   string
		values map[string]any
		want   []string
	}{
		{"code", map[string]any{"reviewer": "engineer"}, []string{e.user("engineer")}},
		{"id", map[string]any{"reviewer": e.user("engineer")}, []string{e.user("engineer")}},
		{"code list", map[string]any{"reviewer": []any{"engineer", "eng-manager"}}, []string{e.user("engineer"), e.user("eng-manager")}},
		{"unknown", map[string]any{"reviewer": "nobody"}, []string{}},
		{"missing", map[string]any{}, []string{}},
	}
	for _, tc := range cases {
		t.Run("user "+tc.name, func(t *testing.T) {
			got := resolve(userStep, tc.values)
			if len(got) != len(tc.want) {
				t.Fatalf("ids=%v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids=%v, want %v", got, tc.want)
				}
			}
		})
	}

	teamStep := model.RouteStep{StepType: model.StepTypeDynamicTeam, Targets: []model.RouteTarget{{Variable: "$approving-team"}}}
	for _, value := range []string{"finance", e.team("finance")} {
		got := resolve(teamStep, map[string]any{"approving-team": value})
		if len(got) != 2 || !pf14Contains(got, e.user("accountant")) || !pf14Contains(got, e.user("finance-manager")) {
			t.Fatalf("team ref %q resolved to %v, want the finance members", value, got)
		}
	}
	if got := resolve(teamStep, map[string]any{}); len(got) != 0 {
		t.Fatalf("missing team variable resolved to %v, want none", got)
	}
}

// PF-10: end to end. A text field holding a user code drives a dynamic_user
// step, and "$"-prefixed variables work in route step conditions.
func TestDynamicUserRouteEndToEnd(t *testing.T) {
	e := pf14Setup(t)
	step := model.RouteStep{Code: "dyn", StepOrder: 1, Title: "dyn", StepType: model.StepTypeDynamicUser, ActionType: model.ActionTypeApprove, Targets: []model.RouteTarget{{Variable: "$purpose"}}}
	compatSetRoute(t, e, "expense-claim", step)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 5000, "purpose": "eng-manager"})
	var id string
	e.apply(func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if got := pf14cCurrentApprover(t, e, id); got != e.user("eng-manager") {
		t.Fatalf("current approver=%s, want eng-manager %s", got, e.user("eng-manager"))
	}
	compatAssertTypes(t, "approve", compatApprove(t, e, "eng-manager", id), model.WebhookEventTicketApproved, model.WebhookEventTicketCompleted)

	conditional := compatUserStep("cond", 1, model.ActionTypeApprove, "finance-manager")
	conditional.RouteStepCondition = &model.RouteStepCondition{
		ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll,
		RouteStepConditionFields: []model.ConditionField{{FieldKey: model.StepConditionFieldKeyNumberVariable, Variable: "$amount", Symbol: "greater_than", Value: 1000}},
	}
	compatSetRoute(t, e, "expense-claim", conditional)
	skippedID, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if got := e.ticket(skippedID).Status; got != model.TicketStatusCompleted {
		t.Fatalf("amount 5000 > 1000 should skip the only step via $amount; status=%s", got)
	}
}
