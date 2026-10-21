package engine

import (
	"testing"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestRestrictedStepConditionReferencesAreKnownEmpty(t *testing.T) {
	e := pf14Setup(t)
	def := model.WorkflowDef{SectionList: []model.WorkflowSection{{SectionType: model.SectionTypeForm, ViewRestriction: true, FormFields: []model.FormField{{ID: "secret-id", Code: "secret"}}}}}
	values := pf14RouteStepValues(def, map[string]any{"secret": "hidden"})
	for _, tc := range []struct {
		symbol string
		want   bool
	}{{"is_empty", true}, {"is_not_empty", false}} {
		field := model.ConditionField{FieldKey: model.StepConditionFieldKeyTextVariable, Variable: "$secret", Symbol: tc.symbol}
		matched, err := pf14bStepConditionMatch(e.ctx, e.db.Q(), &model.Ticket{}, []model.ConditionField{field}, model.StepCombinationAll, values)
		if err != nil || matched != tc.want {
			t.Fatalf("%s matched=%v err=%v, want %v", tc.symbol, matched, err, tc.want)
		}
		// conditional skips on a miss; conditional_skip skips on a match.
		conditionalModeSkips := !matched
		conditionalSkipModeSkips := matched
		if conditionalModeSkips != (tc.symbol == "is_not_empty") || conditionalSkipModeSkips != (tc.symbol == "is_empty") {
			t.Fatalf("conditional mode skip results conditional=%v conditional_skip=%v for %s", conditionalModeSkips, conditionalSkipModeSkips, tc.symbol)
		}
	}

	// Materialize a real route from a non-empty restricted form value and
	// inspect the persisted step statuses for both condition modes.
	e = pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	definition, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	for si := range definition.SectionList {
		for _, field := range definition.SectionList[si].FormFields {
			if field.Code == "purpose" {
				definition.SectionList[si].ViewRestriction = true
			}
		}
	}
	condition := func(kind, symbol string) *model.RouteStepCondition {
		return &model.RouteStepCondition{ConditionType: kind, CombinationType: model.StepCombinationAll, RouteStepConditionFields: []model.ConditionField{{FieldKey: model.StepConditionFieldKeyTextVariable, Variable: "$purpose", Symbol: symbol}}}
	}
	compatSetRoute(t, e, "expense-claim",
		model.RouteStep{Code: "cond-empty", StepOrder: 1, StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove, UserCodes: []string{"ceo"}, RouteStepCondition: condition(model.StepConditionTypeConditional, "is_empty")},
		model.RouteStep{Code: "cond-not-empty", StepOrder: 2, StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove, UserCodes: []string{"ceo"}, RouteStepCondition: condition(model.StepConditionTypeConditional, "is_not_empty")},
		model.RouteStep{Code: "skip-empty", StepOrder: 3, StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove, UserCodes: []string{"ceo"}, RouteStepCondition: condition(model.StepConditionTypeConditionalSkip, "is_empty")},
		model.RouteStep{Code: "skip-not-empty", StepOrder: 4, StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove, UserCodes: []string{"ceo"}, RouteStepCondition: condition(model.StepConditionTypeConditionalSkip, "is_not_empty")})
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 10, "purpose": "restricted value"})
	ticket := e.ticket(id)
	ticket.Status = model.TicketStatusInProgress
	if err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		return e.engine.pf14BuildRoute(e.ctx, tx, &ticket, definition, inputs, nil, store.Millis(e.clock.Now()))
	}); err != nil {
		t.Fatal(err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	wantSkip := map[string]bool{"cond-empty": false, "cond-not-empty": true, "skip-empty": true, "skip-not-empty": false}
	seen := map[string]bool{}
	for _, step := range steps {
		if step.RouteStepCode == nil {
			continue
		}
		seen[*step.RouteStepCode] = true
		if step.Skip != wantSkip[*step.RouteStepCode] {
			t.Errorf("built step %s skip=%v, want %v", *step.RouteStepCode, step.Skip, wantSkip[*step.RouteStepCode])
		}
	}
	if len(seen) != len(wantSkip) {
		t.Fatalf("built route step codes=%v, want %d condition cases", seen, len(wantSkip))
	}
}

func TestReferenceRouteStepConditionsResolveTeamAndMasterSelections(t *testing.T) {
	e := pf14Setup(t)
	if _, err := e.db.Q().ExecContext(e.ctx, `INSERT INTO general_masters(id,code,name,default_sort_by,created_at,updated_at) VALUES('gm-r4','R4_MASTER','Master','code',0,0)`); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, code string }{{"gm-item-a", "A"}, {"gm-item-b", "B"}} {
		if _, err := e.db.Q().ExecContext(e.ctx, `INSERT INTO general_master_items(id,general_master_id,code,name,inputs,input_ids,created_at,updated_at) VALUES(?,?,?,?,'{}','{}',0,0)`, item.id, "gm-r4", item.code, item.code); err != nil {
			t.Fatal(err)
		}
	}
	ticket := model.Ticket{AuthorTeamID: pf14bStringPtr(e.team("sales"))}
	cases := []struct {
		name   string
		field  model.ConditionField
		values map[string]any
		want   bool
	}{
		{"team code", model.ConditionField{FieldKey: model.StepConditionFieldKeyAuthorTeam, Symbol: "equal", Value: "sales"}, nil, true},
		{"master single item", model.ConditionField{FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "product", Symbol: "equal", GeneralMasterCode: "R4_MASTER", GeneralMasterItemCode: "A"}, map[string]any{"product": "gm-item-a"}, true},
		{"master multiple contains selected", model.ConditionField{FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "products", Symbol: "equal", GeneralMasterCode: "R4_MASTER", GeneralMasterItemCode: "B"}, map[string]any{"products": []any{"gm-item-a", "gm-item-b"}}, true},
		{"master multiple missing selection", model.ConditionField{FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "products", Symbol: "equal", GeneralMasterCode: "R4_MASTER", GeneralMasterItemCode: "Z"}, map[string]any{"products": []any{"gm-item-a", "gm-item-b"}}, false},
		{"master empty selection", model.ConditionField{FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "products", Symbol: "is_empty", GeneralMasterCode: "R4_MASTER"}, map[string]any{"products": []any{}}, true},
		{"other variable", model.ConditionField{FieldKey: "other_variable", Variable: "$external", Symbol: "equal", Value: "yes"}, map[string]any{"external": "yes"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := pf14bStepConditionMatch(e.ctx, e.db.Q(), &ticket, []model.ConditionField{tc.field}, model.StepCombinationAll, tc.values)
			if err != nil || matched != tc.want {
				t.Fatalf("matched=%v err=%v, want %v", matched, err, tc.want)
			}
		})
	}
}

func TestManagerSelfApprovalIsFinalFallback(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	grade := model.GradeSymbolEqual
	fallback := model.FallbackSkip
	step := model.RouteStep{Code: "manager-self", StepType: model.StepTypeManager, ActionType: model.ActionTypeApprove, ManagerSelectionType: model.ManagerSelectionClosest, AllowSelfApproval: true, FallbackType: &fallback, Targets: []model.RouteTarget{{GradeSymbol: &grade, GradeCodes: []string{"missing-grade"}}}}
	pf14bSingleStepRoute(t, e, workflow.ID, step)
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 500, "purpose": "self fallback"})
	var id string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil || len(assignees) != 1 || assignees[0].UserID != e.user("sales-rep") || steps[0].Skip {
		t.Fatalf("step=%+v assignees=%+v err=%v, want applicant as active assignee", steps[0], assignees, err)
	}
}

func TestApprovalOnlyTeamCannotCreate(t *testing.T) {
	e := pf14Setup(t)
	teamID := e.team("sales")
	draftID, err := pf14cCreate(t, e, "expense-claim", "sales-rep", map[string]any{"amount": 20, "purpose": "eligibility"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Q().ExecContext(e.ctx, `UPDATE teams SET approve_only=1 WHERE id=?`, teamID); err != nil {
		t.Fatal(err)
	}
	workflow, _ := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	inputs := []FieldInput{}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: teamID, Inputs: &inputs})
		return err
	})
	api, ok := kferr.As(err)
	if !ok || api.Code != "ticket_create_restricted" && api.Code != "validation_failed" {
		t.Fatalf("create error=%v, want application-team rejection", err)
	}
	status := model.TicketStatusInProgress
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Update(e.ctx, tx, e.actor("sales-rep"), draftID, UpdateInput{Status: &status})
		return err
	})
	api, ok = kferr.As(err)
	if !ok || api.Code != "ticket_create_restricted" && api.Code != "validation_failed" {
		t.Fatalf("submit error=%v, want approval-only application-team rejection", err)
	}
}

func TestSendBackReentersTeamStepAndRefreshesAssignments(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim", model.RouteStep{Code: "team-step", StepOrder: 1, StepType: model.StepTypeTeam, ActionType: model.ActionTypeApprove, RequiredApprovalsNumber: 1, ManagerSelectionType: model.ManagerSelectionClosest, Targets: []model.RouteTarget{{TeamCode: "sales"}}}, compatUserStep("last", 2, model.ActionTypeApprove, "ceo"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	teamStep := cleanupStep(t, e, id, "team-step")
	before, err := store.EngStepAssignees(e.ctx, e.db.Q(), teamStep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("precondition: team step has no assignees")
	}
	teamActor := ""
	for code, userID := range e.users {
		if userID == before[0].UserID {
			teamActor = code
			break
		}
	}
	if teamActor == "" {
		t.Fatalf("no fixture code for team assignee %s", before[0].UserID)
	}
	compatApprove(t, e, teamActor, id)
	if _, err = e.db.Q().ExecContext(e.ctx, `DELETE FROM team_memberships WHERE team_id=? AND user_id=?`, e.team("sales"), e.user("sales-manager")); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES('r4-sendback-member',?,?,1,'[]',0,0)`, e.team("sales"), e.user("assistant")); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Hour)
	entryTime := e.clock.Now()
	if _, err = pf15SendBack(e, "ceo", id, 1); err != nil {
		t.Fatal(err)
	}
	teamStep = cleanupStep(t, e, id, "team-step")
	after, err := store.EngStepAssignees(e.ctx, e.db.Q(), teamStep.ID)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := false
	var refreshedAssignee model.TicketAssignee
	for _, assignee := range after {
		if assignee.UserID == e.user("assistant") {
			refreshed, refreshedAssignee = true, assignee
		}
	}
	if teamStep.Status != model.StepStatusInProgress || !teamStep.UpdatedAt.Equal(entryTime) || len(after) == 0 || !refreshed || refreshedAssignee.AssignedAt == nil || !refreshedAssignee.AssignedAt.Equal(entryTime) {
		t.Fatalf("reentered step=%+v assignees=%+v", teamStep, after)
	}
	var viewerCount int
	if err = e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM ticket_viewers WHERE ticket_id=? AND user_id=?`, id, e.user("assistant")).Scan(&viewerCount); err != nil {
		t.Fatal(err)
	}
	if viewerCount == 0 {
		t.Fatal("send-back did not add the newly eligible manager to ticket_viewers")
	}
}

func TestNominationsRequireActiveEligibleUsers(t *testing.T) {
	e := pf14Setup(t)
	if err := pf14bValidateNominee(e.ctx, e.db.Q(), e.user("suspended-user"), nil, nil); err == nil {
		t.Fatal("inactive nominee was accepted")
	}
	// A target restriction excludes an active user outside the target team.
	if err := pf14bValidateNominee(e.ctx, e.db.Q(), e.user("ceo"), []model.RouteTarget{{TeamCode: "sales"}}, nil); err == nil {
		t.Fatal("out-of-team nominee was accepted")
	}
	gradeSymbol := model.GradeSymbolEqual
	if err := pf14bValidateNominee(e.ctx, e.db.Q(), e.user("sales-rep"), []model.RouteTarget{{TeamCode: "sales", GradeSymbol: &gradeSymbol, GradeCodes: []string{"manager"}}}, nil); err == nil {
		t.Fatal("nominee outside target grade was accepted")
	}
}

func TestNominationPathsEnforceActivationAndTargets(t *testing.T) {
	t.Run("draft applicant nomination", func(t *testing.T) {
		e := pf14Setup(t)
		min := 1
		compatSetRoute(t, e, "expense-claim", model.RouteStep{Code: "nominee", StepOrder: 1, StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min, Targets: []model.RouteTarget{{TeamCode: "sales"}}}, compatUserStep("final", 2, model.ActionTypeApprove, "ceo"))
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		_, err := compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "nominee", UserIDs: []string{e.user("engineer")}}}})
		assertPF14ErrorCode(t, err, "validation_failed")
		_, err = compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "nominee", UserIDs: []string{e.user("suspended-user")}}}})
		assertPF14ErrorCode(t, err, "validation_failed")
	})
	t.Run("approver customization", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetRoute(t, e, "expense-claim", compatUserStep("first", 1, model.ActionTypeApprove, "accountant"), model.RouteStep{Code: "custom-next", StepOrder: 2, StepType: model.StepTypeAssigneeCustomizable, ActionType: model.ActionTypeApprove, Targets: []model.RouteTarget{{TeamCode: "sales", Descendants: true}}})
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		_, err := compatUpdate(e, "accountant", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "custom-next", UserIDs: []string{e.user("ceo")}}}})
		assertPF14ErrorCode(t, err, "validation_failed")
		_, err = compatUpdate(e, "accountant", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "custom-next", UserIDs: []string{e.user("sales-deputy")}}}})
		if err != nil {
			t.Fatalf("valid descendant target nominee rejected: %v", err)
		}
	})
}

func TestRouteMaterializationRevalidatesNominationsOnSubmitAndReapply(t *testing.T) {
	newRoute := func(t *testing.T, e *pf14TestEnv) {
		t.Helper()
		min := 1
		compatSetRoute(t, e, "expense-claim", model.RouteStep{Code: "nominee", StepOrder: 1, StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min, Targets: []model.RouteTarget{{TeamCode: "sales"}}}, compatUserStep("final", 2, model.ActionTypeApprove, "ceo"))
	}
	for _, tc := range []struct {
		name   string
		revoke func(*pf14TestEnv)
	}{
		{"candidate becomes inactive", func(e *pf14TestEnv) {
			if _, err := e.db.Q().ExecContext(e.ctx, `UPDATE users SET status='suspended' WHERE id=?`, e.user("sales-manager")); err != nil {
				e.t.Fatal(err)
			}
		}},
		{"candidate leaves target team", func(e *pf14TestEnv) {
			if _, err := e.db.Q().ExecContext(e.ctx, `DELETE FROM team_memberships WHERE team_id=? AND user_id=?`, e.team("sales"), e.user("sales-manager")); err != nil {
				e.t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pf14Setup(t)
			newRoute(t, e)
			id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
			_, err := compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "nominee", UserIDs: []string{e.user("sales-manager")}}}})
			if err != nil {
				t.Fatal(err)
			}
			tc.revoke(e)
			_, err = compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
			assertPF14ErrorCode(t, err, "validation_failed")
		})
	}
	t.Run("reapplication of a rejected ticket", func(t *testing.T) {
		e := pf14Setup(t)
		newRoute(t, e)
		workflow, fields := pf14FormDefinition(t, e, "expense-claim")
		inputs := pf14CompleteInputs(fields, map[string]any{"amount": 10, "purpose": "reapply"})
		var id string
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			var err error
			id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs, Approvers: []ApproverInput{{RouteStepCode: "nominee", UserIDs: []string{e.user("sales-manager")}}}})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pf15SendBack(e, "sales-manager", id, 0); err != nil {
			t.Fatal(err)
		}
		if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE users SET status='suspended' WHERE id=?`, e.user("sales-manager")); err != nil {
			t.Fatal(err)
		}
		_, err = compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
		assertPF14ErrorCode(t, err, "validation_failed")
	})
}

func TestPreviousStepNominationHonorsMinCustomAssignees(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "expense-claim")
	minZero := 0
	compatSetRoute(t, e, "expense-claim", compatUserStep("nominator", 1, model.ActionTypeApprove, "accountant"), model.RouteStep{Code: "previous-nominee", StepOrder: 2, StepType: model.StepTypeAssigneeCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &minZero})
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": 10, "purpose": "optional"})
	var id string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pf15Approve(e, "accountant", id); err != nil {
		t.Fatalf("zero minimum preceding approval: %v", err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if err != nil || len(steps) != 2 || !steps[1].Skip || steps[1].Status != model.StepStatusSkipped {
		t.Fatalf("steps=%+v err=%v, want optional empty nomination skipped", steps, err)
	}
	// The populated case provides one candidate to a later step requiring two.
	minTwo := 2
	compatSetRoute(t, e, "expense-claim", compatUserStep("nominator-two", 1, model.ActionTypeApprove, "accountant"), model.RouteStep{Code: "previous-nominee-two", StepOrder: 2, StepType: model.StepTypeAssigneeCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &minTwo})
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusInProgress, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = compatUpdate(e, "accountant", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "previous-nominee-two", UserIDs: []string{e.user("eng-manager")}}}}); err != nil {
		t.Fatalf("previous approver nomination: %v", err)
	}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("accountant"), id, nil)
		return err
	})
	api, ok := kferr.As(err)
	if !ok || api.Code != "assignment_required_before_approve" {
		t.Fatalf("one nominee preceding approval error=%v, want min-count validation", err)
	}
}
