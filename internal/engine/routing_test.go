package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestPurchaseConditionsAndNomination(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "purchase-request")
	inputs := pf14CompleteInputs(fields, map[string]any{
		"amount": 60000, "item": "Laptop", "urgent": []string{"yes"},
		"reviewer": e.user("engineer"), "approving-team": e.team("platform"),
	})
	status := model.TicketStatusInProgress
	var ticketID string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		ticketID, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{
			Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs,
			Approvers: []ApproverInput{{RouteStepCode: "purchase-nominee", UserIDs: []string{e.user("eng-manager")}}},
		})
		return err
	})
	if err != nil {
		t.Fatalf("create purchase request: %v", err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]model.TicketStep{}
	for _, step := range steps {
		if step.RouteStepCode != nil {
			byCode[*step.RouteStepCode] = step
		}
	}
	if !byCode["purchase-urgent"].Skip || byCode["purchase-urgent"].Status != model.StepStatusSkipped {
		t.Errorf("urgent step should be skipped for urgent=yes: %+v", byCode["purchase-urgent"])
	}
	if byCode["purchase-large"].Status == model.StepStatusSkipped {
		t.Errorf("large amount step should be included for amount=60000: %+v", byCode["purchase-large"])
	}

	missingNomineeErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return err
	})
	if api, ok := kferr.As(missingNomineeErr); !ok || api.Code != "validation_failed" {
		t.Errorf("missing nominee error=%v, want validation_failed", missingNomineeErr)
	}

	budgetErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("eng-manager"), ticketID, nil)
		return err
	})
	if api, ok := kferr.As(budgetErr); !ok || api.Code != "input_required_before_approve" {
		t.Errorf("approve without budget-code error=%v, want input_required_before_approve", budgetErr)
	}
}

func TestLeaveAssigneeCustomizableAndAuthorConfirm(t *testing.T) {
	e := pf14Setup(t)
	workflow, fields := pf14FormDefinition(t, e, "leave-request")
	inputs := pf14CompleteInputs(fields, map[string]any{"start-date": "2025-02-10", "reason": "vacation"})
	status := model.TicketStatusInProgress
	var ticketID string
	err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		ticketID, _, err = e.engine.Create(e.ctx, tx, e.actor("hr-manager"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("hr"), Inputs: &inputs})
		return err
	})
	if err != nil {
		t.Fatalf("create leave request: %v", err)
	}
	steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 2 || steps[0].RouteStepCode == nil || *steps[0].RouteStepCode != "leave-manager" {
		t.Fatalf("leave route steps=%+v, first step should be leave-manager", steps)
	}
	managerAssignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(managerAssignees) != 1 || managerAssignees[0].UserID != e.user("ceo") {
		t.Fatalf("leave manager assignees=%+v, want CEO", managerAssignees)
	}
	assignmentErr := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("ceo"), ticketID, nil)
		return err
	})
	if api, ok := kferr.As(assignmentErr); !ok || api.Code != "assignment_required_before_approve" {
		t.Fatalf("CEO approval without delegate error=%v, want assignment_required_before_approve", assignmentErr)
	}
	delegates := []ApproverInput{{RouteStepCode: "leave-delegate", UserIDs: []string{e.user("hr-manager")}}}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Update(e.ctx, tx, e.actor("ceo"), ticketID, UpdateInput{Approvers: &delegates})
		return err
	}); err != nil {
		t.Fatalf("set delegate: %v", err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("ceo"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("CEO approve after setting delegate: %v", err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Approve(e.ctx, tx, e.actor("hr-manager"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("delegate approve: %v", err)
	}
	steps, err = store.EngTicketSteps(e.ctx, e.db.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 4 || steps[2].Status != model.StepStatusCompleted {
		t.Fatalf("none action step did not auto-complete: %+v", steps)
	}
	var events []model.WebhookEvent
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		events, err = e.engine.Approve(e.ctx, tx, e.actor("hr-manager"), ticketID, nil)
		return err
	}); err != nil {
		t.Fatalf("author confirmation: %v", err)
	}
	if len(events) != 2 || events[0].Type != model.WebhookEventTicketConfirmed || events[1].Type != model.WebhookEventTicketCompleted {
		t.Fatalf("author confirmation events=%+v, want confirmed then completed", events)
	}
	if got := e.ticket(ticketID).Status; got != model.TicketStatusCompleted {
		t.Fatalf("ticket status=%s, want completed", got)
	}
}

func TestStepConditionAndFallback(t *testing.T) {
	e := pf14Setup(t)
	ticket := model.Ticket{AuthorID: pf14bStringPtr(e.user("sales-rep")), AuthorTeamID: pf14bStringPtr(e.team("sales"))}
	gradeFields := []model.ConditionField{{FieldKey: model.StepConditionFieldKeyAuthorGrade, Symbol: "greater_than_or_equal", Value: "staff"}}
	matched, err := pf14bStepConditionMatch(e.ctx, e.db.Q(), &ticket, gradeFields, model.StepCombinationAll, nil)
	if err != nil || !matched {
		t.Fatalf("author grade condition matched=%v err=%v, want true", matched, err)
	}
	childTicket := model.Ticket{AuthorID: ticket.AuthorID, AuthorTeamID: pf14bStringPtr(e.team("sales-east"))}
	teamFields := []model.ConditionField{{FieldKey: model.StepConditionFieldKeyAuthorTeam, Symbol: "descendants_or_equal", Value: e.team("sales")}}
	matched, err = pf14bStepConditionMatch(e.ctx, e.db.Q(), &childTicket, teamFields, model.StepCombinationAll, nil)
	if err != nil || !matched {
		t.Fatalf("child-team ancestry condition matched=%v err=%v, want true", matched, err)
	}
	masterFields := []model.ConditionField{{FieldKey: model.StepConditionFieldKeyGeneralMasterVariable, Variable: "master_item", Symbol: "equal", Value: "item-id"}}
	matched, err = pf14bStepConditionMatch(e.ctx, e.db.Q(), &ticket, masterFields, model.StepCombinationAll, map[string]any{"master_item": "item-id"})
	if err != nil || !matched {
		t.Fatalf("general-master item ID condition matched=%v err=%v, want true", matched, err)
	}
	checkboxFields := []model.ConditionField{{FieldKey: model.StepConditionFieldKeyCheckboxVariable, Variable: "urgent", Symbol: "equal", Value: "yes"}}
	matched, err = pf14bStepConditionMatch(e.ctx, e.db.Q(), &ticket, checkboxFields, model.StepCombinationAll, map[string]any{"urgent": []any{"yes"}})
	if err != nil || !matched {
		t.Fatalf("conditional_skip checkbox condition matched=%v err=%v, want true", matched, err)
	}

	managerTicket := model.Ticket{AuthorID: pf14bStringPtr(e.user("sales-manager")), AuthorTeamID: pf14bStringPtr(e.team("sales"))}
	managerStep := model.RouteStep{StepType: model.StepTypeManager, ManagerSelectionType: model.ManagerSelectionClosest}
	var managerIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		managerIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &managerTicket, managerStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(managerIDs) != 1 || managerIDs[0] != e.user("ceo") {
		t.Fatalf("manager IDs=%v, want parent-team CEO %s", managerIDs, e.user("ceo"))
	}
	allManagerStep := model.RouteStep{StepType: model.StepTypeManager, ManagerSelectionType: model.ManagerSelectionAll}
	var allManagerIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		allManagerIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, allManagerStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pf14Contains(allManagerIDs, e.user("sales-manager")) || !pf14Contains(allManagerIDs, e.user("ceo")) {
		t.Fatalf("all manager IDs=%v, want sales-manager and CEO", allManagerIDs)
	}

	gradeManagerTicket := model.Ticket{AuthorID: pf14bStringPtr(e.user("sales-rep")), AuthorTeamID: pf14bStringPtr(e.team("sales-east"))}
	managerGradeSymbol := model.GradeSymbolGreaterThanOrEqual
	managerGradeStep := model.RouteStep{
		StepType: model.StepTypeManager, ManagerSelectionType: model.ManagerSelectionClosest,
		Targets: []model.RouteTarget{{GradeSymbol: &managerGradeSymbol, GradeCodes: []string{"manager"}}},
	}
	var gradeManagerIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		gradeManagerIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &gradeManagerTicket, managerGradeStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gradeManagerIDs) != 1 || gradeManagerIDs[0] != e.user("sales-manager") || pf14Contains(gradeManagerIDs, e.user("sales-deputy")) {
		t.Fatalf("closest >= manager IDs=%v, want exactly sales-manager, excluding sales-deputy", gradeManagerIDs)
	}
	managerGradeStep.Targets[0].GradeCodes = []string{"executive"}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		gradeManagerIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &gradeManagerTicket, managerGradeStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gradeManagerIDs) != 1 || gradeManagerIDs[0] != e.user("ceo") {
		t.Fatalf("closest >= executive IDs=%v, want exactly ceo", gradeManagerIDs)
	}
	for _, fallbackType := range []string{model.FallbackHigherManager, model.FallbackDirectManager} {
		fallbackType := fallbackType
		managerGradeStep.FallbackType = &fallbackType
		managerGradeStep.Targets[0].GradeCodes = []string{"manager"}
		var fallbackManagerIDs []string
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			var err error
			fallbackManagerIDs, err = pf14bFallbackAssignees(e.ctx, tx, &gradeManagerTicket, managerGradeStep, nil)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if fallbackType == model.FallbackDirectManager {
			if len(fallbackManagerIDs) == 0 || pf14Contains(fallbackManagerIDs, e.user("ceo")) {
				t.Fatalf("direct_manager fallback IDs=%v, want closest leaders", fallbackManagerIDs)
			}
		} else if len(fallbackManagerIDs) != 1 || fallbackManagerIDs[0] != e.user("ceo") {
			t.Fatalf("higher_manager fallback IDs=%v, want strictly higher-grade CEO", fallbackManagerIDs)
		}
	}

	teamStep := model.RouteStep{StepType: model.StepTypeTeam, Targets: []model.RouteTarget{{TeamCode: "engineering", Descendants: true}}}
	var teamIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		teamIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, teamStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pf14Contains(teamIDs, e.user("engineer")) {
		t.Fatalf("descendant target members=%v, want engineer %s", teamIDs, e.user("engineer"))
	}
	teamStep.Targets[0].Descendants = false
	var directIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		directIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, teamStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range directIDs {
		if !pf14Contains(teamIDs, id) {
			t.Fatalf("descendant target omitted direct member %s: direct=%v descendants=%v", id, directIDs, teamIDs)
		}
	}
	gradeSymbol := model.GradeSymbolGreaterThanOrEqual
	teamStep.Targets[0].GradeSymbol = &gradeSymbol
	teamStep.Targets[0].GradeCodes = []string{"manager"}
	gradedIDs := []string{}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		gradedIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, teamStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pf14Contains(gradedIDs, e.user("eng-manager")) || pf14Contains(gradedIDs, e.user("engineer")) {
		t.Fatalf("grade filtered target members=%v, want eng-manager and not engineer", gradedIDs)
	}

	var qualifyingChildID string
	var nonQualifyingChildID string
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		qualifyingGradeID, err := store.GradeIDByCode(e.ctx, tx, "manager")
		if err != nil {
			return err
		}
		nonQualifyingGradeID, err := store.GradeIDByCode(e.ctx, tx, "staff")
		if err != nil {
			return err
		}
		now := store.Millis(e.clock.Now())
		qualifyingChildID = store.NewID()
		nonQualifyingChildID = store.NewID()
		for _, user := range []struct {
			id, code, email string
		}{
			{qualifyingChildID, "pf14b-child-manager", "pf14b-child-manager@example.test"},
			{nonQualifyingChildID, "pf14b-child-staff", "pf14b-child-staff@example.test"},
		} {
			if _, err := tx.ExecContext(e.ctx, `INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, user.id, user.code, user.email, user.code, "Test", model.UserStatusActive, "en", model.UserTypeNormal, "{}", now, now); err != nil {
				return err
			}
		}
		for index, member := range []struct {
			userID, gradeID string
		}{{qualifyingChildID, qualifyingGradeID}, {nonQualifyingChildID, nonQualifyingGradeID}} {
			gradeIDs, err := json.Marshal([]string{member.gradeID})
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(e.ctx, `INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, store.NewID(), e.team("platform"), member.userID, false, string(gradeIDs), now+int64(index)+1, now+int64(index)+1); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert child-only grade test users: %v", err)
	}

	teamStep.Targets[0].Descendants = true
	var descendantGradeIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		descendantGradeIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, teamStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(descendantGradeIDs) != 2 || descendantGradeIDs[0] != e.user("eng-manager") || descendantGradeIDs[1] != qualifyingChildID || pf14Contains(descendantGradeIDs, nonQualifyingChildID) {
		t.Fatalf("descendant grade target IDs=%v, want exactly eng-manager and qualifying child-only member %s", descendantGradeIDs, qualifyingChildID)
	}

	teamStep.Targets[0].Descendants = false
	var directGradeIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		directGradeIDs, err = e.engine.pf14ResolveAssignees(e.ctx, tx, &ticket, teamStep, nil, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(directGradeIDs) != 1 || directGradeIDs[0] != e.user("eng-manager") || pf14Contains(directGradeIDs, qualifyingChildID) {
		t.Fatalf("direct grade target IDs=%v, want exactly eng-manager and exclude child-only member %s", directGradeIDs, qualifyingChildID)
	}

	fallback := model.FallbackHigherTeam
	fallbackStep := model.RouteStep{StepType: model.StepTypeTeam, FallbackType: &fallback, Targets: []model.RouteTarget{{TeamCode: "sales-east"}}}
	var fallbackIDs []string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		fallbackIDs, err = pf14bFallbackAssignees(e.ctx, tx, &ticket, fallbackStep, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fallbackIDs) == 0 {
		t.Fatal("higher_team fallback returned no parent-team members")
	}
	if pf14Contains(fallbackIDs, e.user("sales-rep")) {
		t.Fatalf("higher_team fallback retained author despite self-approval being disabled: %v", fallbackIDs)
	}
	for _, fallbackType := range []string{model.FallbackDirectManager, model.FallbackHigherManager} {
		fallbackType := fallbackType
		fallbackManagerStep := model.RouteStep{StepType: model.StepTypeTeam, FallbackType: &fallbackType}
		var ids []string
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			var err error
			ids, err = pf14bFallbackAssignees(e.ctx, tx, &ticket, fallbackManagerStep, nil)
			return err
		})
		if err != nil || !pf14Contains(ids, e.user("sales-manager")) {
			t.Fatalf("%s fallback IDs=%v err=%v, want sales-manager", fallbackType, ids, err)
		}
	}

	t.Run("conditional skip", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "purchase-request")
		route := pf14bSingleStepRoute(t, e, workflow.ID, model.RouteStep{
			Code: "condition-skip", StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove,
			UserCodes:          []string{"sales-manager"},
			RouteStepCondition: &model.RouteStepCondition{ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll, RouteStepConditionFields: []model.ConditionField{{Variable: "urgent", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}}},
		})
		inputs := pf14CompleteInputs(fields, map[string]any{"amount": 60000, "item": "Laptop", "urgent": []string{"yes"}, "reviewer": e.user("engineer"), "approving-team": e.team("platform")})
		status := model.TicketStatusInProgress
		var id string
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			var err error
			id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
		if err != nil || len(steps) != 1 || steps[0].Status != model.StepStatusSkipped || !steps[0].Skip {
			t.Fatalf("conditional skip steps=%+v err=%v", steps, err)
		}
		_ = route
	})

	t.Run("ID-only inputs validate and match numeric step condition", func(t *testing.T) {
		e := pf14Setup(t)
		workflow, fields := pf14FormDefinition(t, e, "expense-claim")
		pf14bSingleStepRoute(t, e, workflow.ID, model.RouteStep{
			Code: "id-only-amount", StepType: model.StepTypeUser, ActionType: model.ActionTypeApprove,
			UserCodes: []string{"sales-manager"},
			RouteStepCondition: &model.RouteStepCondition{
				ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll,
				RouteStepConditionFields: []model.ConditionField{{
					Variable: "amount", FieldKey: model.StepConditionFieldKeyNumberVariable,
					Symbol: "greater_than", Value: 50000,
				}},
			},
		})
		codeInputs := pf14CompleteInputs(fields, map[string]any{"amount": 60000, "purpose": "equipment"})
		idInputs := append([]FieldInput(nil), codeInputs...)
		for i := range idInputs {
			idInputs[i].FormFieldCode = ""
		}
		create := func(inputs []FieldInput) string {
			t.Helper()
			var id string
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				var err error
				id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{
					Status: model.TicketStatusInProgress, WorkflowID: workflow.ID,
					AuthorTeamID: e.team("sales"), Inputs: &inputs,
				})
				return err
			})
			if err != nil {
				t.Fatalf("create ticket: %v", err)
			}
			return id
		}
		codeID, idOnlyID := create(codeInputs), create(idInputs)
		codeTicket, err := store.EngLoadTicket(e.ctx, e.db.Q(), codeID)
		if err != nil {
			t.Fatal(err)
		}
		idOnlyTicket, err := store.EngLoadTicket(e.ctx, e.db.Q(), idOnlyID)
		if err != nil {
			t.Fatal(err)
		}
		if codeTicket.RouteID == nil || idOnlyTicket.RouteID == nil || *codeTicket.RouteID != *idOnlyTicket.RouteID {
			t.Fatalf("resolved routes differ: code=%+v ID-only=%+v", codeTicket.RouteID, idOnlyTicket.RouteID)
		}
		codeSteps, err := store.EngTicketSteps(e.ctx, e.db.Q(), codeID)
		if err != nil {
			t.Fatal(err)
		}
		idOnlySteps, err := store.EngTicketSteps(e.ctx, e.db.Q(), idOnlyID)
		if err != nil {
			t.Fatal(err)
		}
		if len(codeSteps) != 1 || len(idOnlySteps) != 1 || codeSteps[0].Status != model.StepStatusSkipped || idOnlySteps[0].Status != codeSteps[0].Status {
			t.Fatalf("conditional steps differ: code=%+v ID-only=%+v", codeSteps, idOnlySteps)
		}

		missingPurpose := append([]FieldInput(nil), idInputs...)
		for i := range missingPurpose {
			if missingPurpose[i].FormFieldID == fields["purpose"] {
				missingPurpose[i].Value = json.RawMessage(`""`)
			}
		}
		var before int
		if err := e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM tickets`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, _, err := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{
				Status: model.TicketStatusInProgress, WorkflowID: workflow.ID,
				AuthorTeamID: e.team("sales"), Inputs: &missingPurpose,
			})
			return err
		})
		api, ok := kferr.As(err)
		if !ok || api.Code != "validation_failed" || len(api.Errors["purpose"]) == 0 || api.Errors["purpose"][0] != "is required" {
			t.Fatalf("missing ID-only purpose error=%v, want validation_failed errors[purpose]=[is required]", err)
		}
		var after int
		if err := e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM tickets`).Scan(&after); err != nil || after != before {
			t.Fatalf("failed ID-only create persisted a ticket: before=%d after=%d err=%v", before, after, err)
		}
	})

	t.Run("author customizable minimum assignees", func(t *testing.T) {
		minZero, minTwo := 0, 2
		condition := &model.RouteStepCondition{
			ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll,
			RouteStepConditionFields: []model.ConditionField{{Variable: "urgent", FieldKey: model.StepConditionFieldKeyCheckboxVariable, Symbol: "equal", Value: "yes"}},
		}
		cases := []struct {
			name      string
			min       *int
			condition *model.RouteStepCondition
			nominee   bool
			wantError bool
		}{
			{name: "conditional skip ignores missing nominees", condition: condition},
			{name: "conditional skip omits nominees", condition: condition, nominee: true},
			{name: "zero minimum skips without nominees", min: &minZero},
			{name: "minimum two rejects one nominee", min: &minTwo, nominee: true, wantError: true},
			{name: "default minimum one remains required", wantError: true},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				e := pf14Setup(t)
				workflow, fields := pf14FormDefinition(t, e, "purchase-request")
				pf14bSingleStepRoute(t, e, workflow.ID, model.RouteStep{
					Code: "purchase-nominee", StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove,
					MinCustomAssignees: tc.min, RouteStepCondition: tc.condition,
				})
				inputs := pf14CompleteInputs(fields, map[string]any{"amount": 60000, "item": "Laptop", "urgent": []string{"yes"}, "reviewer": e.user("engineer"), "approving-team": e.team("platform")})
				status := model.TicketStatusInProgress
				before := 0
				if err := e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM tickets`).Scan(&before); err != nil {
					t.Fatal(err)
				}
				var nominees []string
				if tc.nominee {
					nominees = []string{e.user("eng-manager")}
				}
				var id string
				err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
					var err error
					id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{
						Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs,
						Approvers: []ApproverInput{{RouteStepCode: "purchase-nominee", UserIDs: nominees}},
					})
					return err
				})
				if tc.wantError {
					api, ok := kferr.As(err)
					if !ok || api.Status != 422 || api.Code != "validation_failed" || len(api.Errors["base"]) != 1 || api.Errors["base"][0] != "custom approver is required" {
						t.Fatalf("create error=%+v, want 422 validation_failed errors.base=[custom approver is required]", err)
					}
					after := 0
					if scanErr := e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM tickets`).Scan(&after); scanErr != nil || after != before {
						t.Fatalf("failed submit persisted ticket: before=%d after=%d err=%v", before, after, scanErr)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
				if err != nil || len(steps) != 1 || steps[0].Status != model.StepStatusSkipped || !steps[0].Skip {
					t.Fatalf("skipped author-customizable steps=%+v err=%v, want status skipped and skip=true", steps, err)
				}
				assignees, err := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
				if err != nil || len(assignees) != 0 {
					t.Fatalf("skipped author-customizable assignees=%+v err=%v, want none", assignees, err)
				}
			})
		}
	})

	for _, fallbackType := range []string{model.FallbackSkip, model.FallbackNoFallback} {
		t.Run(fallbackType, func(t *testing.T) {
			e := pf14Setup(t)
			workflow, fields := pf14FormDefinition(t, e, "expense-claim")
			gradeSymbol := model.GradeSymbolEqual
			step := model.RouteStep{Code: "fallback-manager", StepType: model.StepTypeManager, ActionType: model.ActionTypeApprove, ManagerSelectionType: model.ManagerSelectionClosest, FallbackType: &fallbackType, Targets: []model.RouteTarget{{GradeSymbol: &gradeSymbol, GradeCodes: []string{"missing-grade"}}}}
			pf14bSingleStepRoute(t, e, workflow.ID, step)
			inputs := pf14CompleteInputs(fields, map[string]any{"amount": 5000, "purpose": "fallback"})
			status := model.TicketStatusInProgress
			before := 0
			if err := e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM tickets`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			var id string
			err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
				var err error
				id, _, err = e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if fallbackType == model.FallbackNoFallback {
				if e.ticket(id).Status != model.TicketStatusInProgress || e.ticket(id).CurrentStep != 1 {
					t.Fatalf("ticket=%+v, want in-progress and waiting on empty step", e.ticket(id))
				}
				steps, loadErr := store.EngTicketSteps(e.ctx, e.db.Q(), id)
				if loadErr != nil || len(steps) != 1 || steps[0].Status != model.StepStatusInProgress {
					t.Fatalf("no_fallback steps=%+v err=%v, want active empty step", steps, loadErr)
				}
				assignees, loadErr := store.EngStepAssignees(e.ctx, e.db.Q(), steps[0].ID)
				if loadErr != nil || len(assignees) != 0 {
					t.Fatalf("no_fallback assignees=%+v err=%v, want none", assignees, loadErr)
				}
				return
			}
			steps, err := store.EngTicketSteps(e.ctx, e.db.Q(), id)
			if err != nil || len(steps) != 1 || steps[0].Status != model.StepStatusSkipped || !steps[0].Fallback || steps[0].FallbackResult == nil || *steps[0].FallbackResult != model.FallbackSkip {
				t.Fatalf("fallback skip steps=%+v err=%v", steps, err)
			}
		})
	}
}

func TestEmptyNoFallbackParallelStepKeepsTicketWaiting(t *testing.T) {
	e := pf14Setup(t)
	gradeSymbol := model.GradeSymbolEqual
	fallback := model.FallbackNoFallback
	empty := model.RouteStep{Code: "empty-team", StepOrder: 1, StepType: model.StepTypeManager, ActionType: model.ActionTypeNone, ManagerSelectionType: model.ManagerSelectionClosest, FallbackType: &fallback, Targets: []model.RouteTarget{{GradeSymbol: &gradeSymbol, GradeCodes: []string{"missing-grade"}}}}
	assigned := compatUserStep("assigned", 1, model.ActionTypeApprove, "sales-manager")
	compatSetRoute(t, e, "expense-claim", empty, assigned)
	id, events := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if len(events) == 0 {
		t.Fatal("ticket creation returned no lifecycle events")
	}
	compatApprove(t, e, "sales-manager", id)
	if ticket := e.ticket(id); ticket.Status != model.TicketStatusInProgress || ticket.CurrentStep != 1 {
		t.Fatalf("ticket after parallel approval=%+v, want still waiting on empty step", ticket)
	}
	steps, loadErr := store.EngTicketSteps(e.ctx, e.db.Q(), id)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	for _, step := range steps {
		if step.RouteStepCode != nil && *step.RouteStepCode == "empty-team" && step.Status != model.StepStatusInProgress {
			t.Fatalf("empty step status=%s, want in_progress", step.Status)
		}
	}
}

func pf14bStringPtr(value string) *string { return &value }

func pf14bSingleStepRoute(t *testing.T, e *pf14TestEnv, workflowID string, step model.RouteStep) model.RouteDef {
	t.Helper()
	workflow, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	routeID, err := store.RouteIDByCode(e.ctx, e.db.Q(), workflow.RouteCode)
	if err != nil {
		t.Fatal(err)
	}
	route, err := store.RouteDefByID(e.ctx, e.db.Q(), routeID)
	if err != nil {
		t.Fatal(err)
	}
	step.ID = "pf14b-test-step"
	step.StepOrder = 1
	route.Steps = []model.RouteStep{step}
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE routes SET definition_json=? WHERE id=?`, string(data), route.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return route
}
