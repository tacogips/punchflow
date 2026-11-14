package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestR5ManagerSelfApprovalDoesNotOverrideExecutionSkip(t *testing.T) {
	e := pf14Setup(t)
	fallback := model.FallbackSkip
	gradeSymbol := model.GradeSymbolEqual
	step := model.RouteStep{
		Code: "manager-self-skip", StepOrder: 1, StepType: model.StepTypeManager,
		ActionType: model.ActionTypeApprove, ManagerSelectionType: model.ManagerSelectionClosest,
		AllowSelfApproval: true, FallbackType: &fallback,
		Targets: []model.RouteTarget{{GradeSymbol: &gradeSymbol, GradeCodes: []string{"missing-grade"}}},
		RouteStepCondition: &model.RouteStepCondition{
			ConditionType:            model.StepConditionTypeConditionalSkip,
			CombinationType:          model.StepCombinationAll,
			RouteStepConditionFields: []model.ConditionField{{FieldKey: model.StepConditionFieldKeyNumberVariable, Variable: "$amount", Symbol: "greater_than", Value: 1000}},
		},
	}
	compatSetRoute(t, e, "expense-claim", step)
	id, _, err := compatCreateExpense(t, e, "sales-rep", "sales", model.TicketStatusInProgress, nil)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil || len(steps) != 1 || !steps[0].Skip || steps[0].Status != model.StepStatusSkipped {
		t.Fatalf("steps=%+v err=%v, want execution-condition skip", steps, err)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil || len(assignees) != 0 {
		t.Fatalf("assignees=%+v err=%v, want none", assignees, err)
	}
}

func TestR5NominationMinimumCountsDistinctUsers(t *testing.T) {
	e := pf14Setup(t)
	min := 2
	step := model.RouteStep{Code: "applicants", StepOrder: 1, StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min}
	compatSetRoute(t, e, "expense-claim", step)
	_, _, err := compatCreateExpense(t, e, "sales-rep", "sales", model.TicketStatusInProgress, []ApproverInput{{RouteStepCode: "applicants", UserIDs: []string{e.user("sales-manager"), e.user("sales-manager")}}})
	assertPF14ErrorCode(t, err, "validation_failed")
}

func TestR5PreviousApproverDuplicateNominationsDoNotMeetMinimum(t *testing.T) {
	e := pf14Setup(t)
	min := 2
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("first", 1, model.ActionTypeApprove, "accountant"),
		model.RouteStep{Code: "previous-nominees", StepOrder: 2, StepType: model.StepTypeAssigneeCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min},
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	nominations := []ApproverInput{{RouteStepCode: "previous-nominees", UserIDs: []string{e.user("sales-manager"), e.user("sales-manager")}}}
	if _, err := compatUpdate(e, "accountant", id, UpdateInput{Approvers: &nominations}); err != nil {
		t.Fatal(err)
	}
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("accountant"), id, nil)
		return err
	})
	assertPF14ErrorCode(t, err, "assignment_required_before_approve")
}

func TestR5DirectApplicationCannotNominatePrecedingStepAuthority(t *testing.T) {
	e := pf14Setup(t)
	min := 1
	compatSetRoute(t, e, "expense-claim",
		compatUserStep("first", 1, model.ActionTypeApprove, "accountant"),
		model.RouteStep{Code: "previous-nominates", StepOrder: 2, StepType: model.StepTypeAssigneeCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min},
	)
	_, _, err := compatCreateExpense(t, e, "sales-rep", "sales", model.TicketStatusInProgress, []ApproverInput{{RouteStepCode: "previous-nominates", UserIDs: []string{e.user("sales-manager")}}})
	assertPF14ErrorCode(t, err, "invalid_parameter")
}

func TestR5ProxyDraftSurvivesRegistrationEdits(t *testing.T) {
	e := pf14Setup(t)
	clientID := e.user("ceo")
	var applicantTeamID string
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT m.team_id FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE m.user_id=? AND c.current=1 ORDER BY m.team_id LIMIT 1`, clientID).Scan(&applicantTeamID); err != nil {
		t.Fatal(err)
	}
	applicantTeam, err := store.GetTeam(e.ctx, e.db.Q(), applicantTeamID)
	if err != nil {
		t.Fatal(err)
	}
	compatSetWorkflow(t, e, "expense-claim", "$.availableToEveryone", false)
	compatSetWorkflow(t, e, "expense-claim", "$.availableTeams", []model.WorkflowTeam{{TeamCode: applicantTeam.Code}})
	var proxyMemberCount int
	if err = e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM team_memberships WHERE user_id=? AND team_id=?`, e.user("assistant"), applicantTeamID).Scan(&proxyMemberCount); err != nil || proxyMemberCount != 0 {
		t.Fatalf("test requires proxy outside applicant team: memberships=%d err=%v", proxyMemberCount, err)
	}
	id, err := pf14cCreate(t, e, "expense-claim", "assistant", map[string]any{"amount": 250, "purpose": "proxy draft"}, &clientID)
	if err != nil {
		t.Fatal(err)
	}
	var memberships int
	if err = e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM team_memberships WHERE user_id=? AND team_id=?`, clientID, *e.ticket(id).AuthorTeamID).Scan(&memberships); err != nil || memberships == 0 {
		t.Fatalf("selected application team is not the proxy client's team: memberships=%d err=%v", memberships, err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE proxy_applicants SET workflow_ids='["different-workflow"]' WHERE user_id=? AND proxy_user_id=?`, clientID, e.user("assistant")); err != nil {
		t.Fatal(err)
	}
	if _, err = compatUpdate(e, "assistant", id, UpdateInput{Title: pf14bStringPtr("edited after registration change")}); err != nil {
		t.Fatalf("existing proxy draft update rejected after settings edit: %v", err)
	}
	if _, err = compatUpdate(e, "assistant", id, UpdateInput{Status: pf14bStringPtr(model.TicketStatusInProgress)}); err != nil {
		t.Fatalf("existing proxy draft submission rejected after settings edit: %v", err)
	}
}
