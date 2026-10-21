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

func TestRestrictedFieldsDoNotDriveRouteStepAssigneesOrConditions(t *testing.T) {
	e := pf14Setup(t)
	const sectionID = "00000000-0000-4000-8000-0000000000d1"
	const reviewerID = "00000000-0000-4000-8000-0000000000d2"
	const flagID = "00000000-0000-4000-8000-0000000000d3"
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		def.SectionList = append(def.SectionList, model.WorkflowSection{
			ID: sectionID, SectionType: model.SectionTypeForm, ViewRestriction: true, AllowViewAuthor: true,
			FormFields: []model.FormField{{ID: reviewerID, Code: "restricted_reviewer", FieldType: "text", Author: true}, {ID: flagID, Code: "restricted_flag", FieldType: "text", Author: true}},
		})
	})
	dynamic := model.RouteStep{Code: "restricted-dynamic", StepOrder: 1, Title: "Dynamic", StepType: model.StepTypeDynamicUser, ActionType: model.ActionTypeApprove, Targets: []model.RouteTarget{{Variable: "$restricted_reviewer"}}, FallbackType: compatStr(model.FallbackSkip)}
	conditional := compatUserStep("restricted-condition", 2, model.ActionTypeApprove, "finance-manager")
	conditional.RouteStepCondition = &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditional, CombinationType: model.StepCombinationAll,
		RouteStepConditionFields: []model.ConditionField{{FieldKey: model.StepConditionFieldKeyTextVariable, Variable: "$restricted_flag", Symbol: "equal", Value: "true"}},
	}
	compatSetRoute(t, e, "expense-claim", dynamic, conditional)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	delete(fields, "restricted_reviewer")
	delete(fields, "restricted_flag")
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 5000, "purpose": "restricted-routing"})
	inputs = append(inputs, pf14Input("restricted_reviewer", reviewerID, "engineer"), pf14Input("restricted_flag", flagID, "true"))
	routeDef := model.WorkflowDef{SectionList: []model.WorkflowSection{{ID: sectionID, SectionType: model.SectionTypeForm, ViewRestriction: true, FormFields: []model.FormField{{ID: reviewerID, Code: "restricted_reviewer"}, {ID: flagID, Code: "restricted_flag"}}}}}
	stepValues := pf14RouteStepValues(routeDef, pf14InputValues(routeDef, inputs))
	var dynamicIDs []string
	e.apply(func(tx *store.Tx) error {
		var err error
		dynamicIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &model.Ticket{AuthorID: compatStr(e.user("sales-rep")), AuthorTeamID: compatStr(e.team("sales"))}, dynamic, stepValues, nil)
		return err
	})
	if len(dynamicIDs) != 0 {
		t.Fatalf("restricted dynamic assignment=%v, want no assignees", dynamicIDs)
	}
	var id string
	e.apply(func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("ticket status=%s, want completed because both restricted-driven steps are skipped", got)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Status != model.StepStatusSkipped || steps[1].Status != model.StepStatusSkipped {
		t.Fatalf("step statuses=%+v, want both skipped", steps)
	}
	for _, step := range steps {
		assignees, err := store.TicketAssignees(e.ctx, e.db.Q(), step.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, assignee := range assignees {
			if assignee.UserID == e.user("engineer") {
				t.Fatalf("restricted dynamic value assigned step %v to %+v", step.RouteStepCode, assignees)
			}
		}
	}
}

// INF-LIFE-R3-03 and INF-LIFE-R3-04: automatic team candidates must be active,
// team assignments refresh on step entry, and existing assignments survive suspension.
func TestTeamAssignmentsUseActiveUsersAndCurrentChartOnEntry(t *testing.T) {
	e := pf14Setup(t)
	first := compatUserStep("first", 1, model.ActionTypeApprove, "accountant")
	teamStep := model.RouteStep{Code: "team-review", StepOrder: 2, Title: "Team review", StepType: model.StepTypeTeam, ActionType: model.ActionTypeApprove, RequiredApprovalsPercent: 100, Targets: []model.RouteTarget{{TeamCode: "finance"}}}
	compatSetRoute(t, e, "expense-claim", first, teamStep)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	var teamTicketStep model.TicketStep
	for _, step := range steps {
		if step.RouteStepCode != nil && *step.RouteStepCode == "team-review" {
			teamTicketStep = step
		}
	}
	if teamTicketStep.ID == "" {
		t.Fatal("team step missing")
	}
	teamID := e.team("finance")
	if _, err = e.db.Q().ExecContext(e.ctx, `DELETE FROM team_memberships WHERE team_id=?`, teamID); err != nil {
		t.Fatal(err)
	}
	admin := e.user("admin")
	if err = store.TeamMembershipInsert(e.ctx, e.db.Q(), model.TeamMembership{ID: store.NewID(), TeamID: teamID, UserID: admin, GradeIDs: []string{}, CreatedAt: e.clock.Now(), UpdatedAt: e.clock.Now()}); err != nil {
		t.Fatal(err)
	}
	compatApprove(t, e, "accountant", id)
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), teamTicketStep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].UserID != admin {
		t.Fatalf("recalculated team assignees=%v, want current member %s", assignees, admin)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, model.UserStatusSuspended, admin); err != nil {
		t.Fatal(err)
	}
	assignees, err = store.EngStepAssignees(e.ctx, e.db.Q(), teamTicketStep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].UserID != admin {
		t.Fatalf("suspension removed existing assignment: %v", assignees)
	}
}

func TestAutomaticTeamAssignmentExcludesSuspendedAndCopiesInstruction(t *testing.T) {
	e := pf14Setup(t)
	if _, err := e.db.Q().ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, model.UserStatusSuspended, e.user("finance-manager")); err != nil {
		t.Fatal(err)
	}
	instruction := "Review this request"
	step := model.RouteStep{Code: "team-review", StepOrder: 1, Title: "Team review", Instruction: &instruction, StepType: model.StepTypeTeam, ActionType: model.ActionTypeApprove, RequiredApprovalsPercent: 100, Targets: []model.RouteTarget{{TeamCode: "finance"}}}
	compatSetRoute(t, e, "expense-claim", step)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Instruction == nil || *steps[0].Instruction != instruction {
		t.Fatalf("ticket steps=%+v, instruction lost", steps)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].UserID != e.user("accountant") {
		t.Fatalf("team assignees=%v, want active accountant only", assignees)
	}
}

// INF-LIFE-R3-02: higher-team fallback searches successive ancestors until the
// configured grade is found, and successful fallback is recorded on the step.
func TestHigherTeamFallbackWalksAncestorsAndRecordsFallback(t *testing.T) {
	e := pf14Setup(t)
	grade := model.GradeSymbolEqual
	fallback := model.FallbackHigherTeam
	step := model.RouteStep{Code: "higher-team", StepOrder: 1, Title: "Higher team", StepType: model.StepTypeTeam, ActionType: model.ActionTypeApprove, RequiredApprovalsPercent: 100, FallbackType: &fallback, Targets: []model.RouteTarget{{TeamCode: "sales-east", GradeSymbol: &grade, GradeCodes: []string{"executive"}}}}
	compatSetRoute(t, e, "expense-claim", step)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || !steps[0].Fallback || steps[0].FallbackResult == nil || *steps[0].FallbackResult != fallback {
		t.Fatalf("step fallback state=%+v", steps)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].UserID != e.user("ceo") {
		t.Fatalf("higher-team fallback assignees=%v, want ancestor CEO", assignees)
	}
}

func TestManagerAssignmentsKeepApplicationSnapshot(t *testing.T) {
	e := pf14Setup(t)
	first := compatUserStep("first", 1, model.ActionTypeApprove, "accountant")
	managerStep := model.RouteStep{Code: "manager-review", StepOrder: 2, Title: "Manager review", StepType: model.StepTypeManager, ManagerSelectionType: model.ManagerSelectionClosest, ActionType: model.ActionTypeApprove, RequiredApprovalsPercent: 100}
	compatSetRoute(t, e, "expense-claim", first, managerStep)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	var managerTicketStep model.TicketStep
	for _, step := range steps {
		if step.RouteStepCode != nil && *step.RouteStepCode == "manager-review" {
			managerTicketStep = step
		}
	}
	if managerTicketStep.ID == "" {
		t.Fatal("manager step missing")
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `DELETE FROM team_memberships WHERE team_id=?`, e.team("sales")); err != nil {
		t.Fatal(err)
	}
	compatApprove(t, e, "accountant", id)
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), managerTicketStep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignees) != 1 || assignees[0].UserID != e.user("sales-manager") {
		t.Fatalf("manager assignees=%v, want application-time sales manager", assignees)
	}
}
