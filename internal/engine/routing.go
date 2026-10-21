package engine

import (
	"context"
	"fmt"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14SelectRouteCode picks the route code for the given form values by walking
// the workflow's route conditions in order.
func pf14SelectRouteCode(def model.WorkflowDef, values map[string]any) (string, error) {
	routeCode := def.RouteCode
	matchedBranch := false
	for _, condition := range def.RouteConditions {
		if condition.ConditionType == model.RouteConditionTypeFieldOtherwise {
			if !matchedBranch && condition.RouteCode != nil {
				routeCode = *condition.RouteCode
			}
			break
		}
		match := condition.ConditionType == model.RouteConditionTypeAlways || pf14ConditionMatchExpression(condition.ConditionFields, condition.CombinationType, condition.CombinationExpression, values)
		if !match {
			continue
		}
		matchedBranch = true
		if condition.ErrorMessage != nil && *condition.ErrorMessage != "" {
			return "", kferr.ValidationFailed(map[string][]string{"base": {*condition.ErrorMessage}})
		}
		if condition.RouteCode != nil {
			routeCode = *condition.RouteCode
			break
		}
	}
	return routeCode, nil
}

// pf14BuildRoute materializes the route steps for a submitted ticket and then
// runs the shared advancement routine (pf14Advance), so skipped groups and
// steps that need no user action are traversed immediately. After it returns
// the ticket is either waiting on the first actionable group or, when nothing
// is left to do, already completed; callers must check ticket.Status.
func (e *Engine) pf14BuildRoute(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, inputs []FieldInput, approvers []ApproverInput, nowTime int64) error {
	values := pf14InputValues(def, inputs)
	routeCode, err := pf14SelectRouteCode(def, values)
	if err != nil {
		return err
	}
	routeID, err := store.RouteIDByCode(ctx, tx, routeCode)
	if err != nil {
		return kferr.ValidationFailed(map[string][]string{"route": {"is unavailable"}})
	}
	route, err := store.RouteDefByID(ctx, tx, routeID)
	if err != nil {
		return err
	}
	if route.Status != model.RouteStatusVisible {
		return kferr.ValidationFailed(map[string][]string{"route": {"is unavailable"}})
	}
	// INF-SEC-05: route selection uses applicant-visible values, while approval
	// step conditions and dynamic targets always treat restricted fields as empty.
	stepValues := pf14RouteStepValues(def, values)
	ticket.RouteID = &routeID
	for _, custom := range approvers {
		if err = store.EngInsertCustomApprovers(ctx, tx, ticket.ID, custom.RouteStepCode, custom.UserIDs); err != nil {
			return err
		}
	}
	display := 0
	groups := map[int][]model.TicketStep{}
	for _, step := range route.Steps {
		display++
		conditionMatches := true
		var er error
		if step.RouteStepCondition != nil && step.RouteStepCondition.ConditionType != model.StepConditionTypeAlways {
			conditionMatches, er = pf14bStepConditionMatch(ctx, tx, ticket, step.RouteStepCondition.RouteStepConditionFields, step.RouteStepCondition.CombinationType, stepValues)
			if er != nil {
				return er
			}
		}
		skip := step.RouteStepCondition != nil && (step.RouteStepCondition.ConditionType == model.StepConditionTypeConditionalSkip && conditionMatches || step.RouteStepCondition.ConditionType == model.StepConditionTypeConditional && !conditionMatches)
		fb := false
		result := ""
		userIDs, er := e.pf14ResolveAssignees(ctx, tx, ticket, step, stepValues, approvers)
		if er != nil {
			return er
		}
		if step.StepType == model.StepTypeAuthorCustomizable && !skip {
			minCustomAssignees := 1
			if step.MinCustomAssignees != nil {
				minCustomAssignees = *step.MinCustomAssignees
			}
			if len(userIDs) < minCustomAssignees {
				return kferr.ValidationFailed(map[string][]string{"base": {"custom approver is required"}})
			}
			if len(userIDs) == 0 {
				skip = true
			}
		}
		if len(userIDs) == 0 && !skip && step.StepType != model.StepTypeAuthorCustomizable && step.StepType != model.StepTypeAssigneeCustomizable {
			if step.FallbackType == nil {
				return kferr.ValidationFailed(map[string][]string{"base": {"step has no assignees"}})
			}
			userIDs, er = pf14bFallbackAssignees(ctx, tx, ticket, step, stepValues)
			if er != nil {
				return er
			}
			if len(userIDs) == 0 {
				fb, result = true, *step.FallbackType
				switch *step.FallbackType {
				case model.FallbackSkip:
					skip = true
				case model.FallbackNoFallback:
					// Keep this step in progress without assignees. An administrator
					// can assign it later; the empty group must keep waiting.
				}
			}
		}
		status := model.StepStatusPending
		if skip {
			status = model.StepStatusSkipped
		}
		routeStepID, routeCodeValue := step.ID, step.Code
		title, action := step.Title, step.ActionType
		ts := model.TicketStep{ID: store.NewID(), TicketID: ticket.ID, RouteStepID: &routeStepID, RouteStepCode: &routeCodeValue, StepOrder: step.StepOrder, DisplayOrder: display, Title: &title, ActionType: action, RequiredApprovalsNumber: step.RequiredApprovalsNumber, RequiredApprovalsPercent: step.RequiredApprovalsPercent, Status: status, Skip: skip, Fallback: fb, CreatedAt: store.Time(nowTime), UpdatedAt: store.Time(nowTime)}
		if fb {
			ts.FallbackResult = &result
		}
		if err = store.EngInsertStep(ctx, tx, ts); err != nil {
			return err
		}
		groups[step.StepOrder] = append(groups[step.StepOrder], ts)
		if skip && step.StepType == model.StepTypeAuthorCustomizable {
			continue
		}
		for _, uid := range userIDs {
			as := model.TicketAssignee{ID: store.NewID(), TicketStepID: ts.ID, TicketID: ticket.ID, UserID: uid, CreatedAt: store.Time(nowTime), UpdatedAt: store.Time(nowTime)}
			if err = store.EngInsertAssignee(ctx, tx, as); err != nil {
				return err
			}
		}
	}
	if len(groups) == 0 {
		return kferr.ValidationFailed(map[string][]string{"route": {"has no steps"}})
	}
	ticket.CurrentStep = 0
	return e.pf14Advance(ctx, tx, ticket, store.Time(nowTime))
}

func pf14RouteStepValues(def model.WorkflowDef, values map[string]any) map[string]any {
	filtered := make(map[string]any, len(values))
	for key, value := range values {
		filtered[key] = value
	}
	for _, section := range def.SectionList {
		if section.SectionType != model.SectionTypeForm || !section.ViewRestriction {
			continue
		}
		for _, field := range section.FormFields {
			delete(filtered, field.ID)
			delete(filtered, field.Code)
			delete(filtered, model.VariableName(field.Code))
		}
	}
	return filtered
}

func (e *Engine) pf14ResolveAssignees(ctx context.Context, tx *store.Tx, ticket *model.Ticket, step model.RouteStep, values map[string]any, approvers []ApproverInput) ([]string, error) {
	ids := []string{}
	add := func(id string) {
		if id != "" && !pf14Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	switch step.StepType {
	case model.StepTypeAuthor:
		if ticket.AuthorID != nil {
			add(*ticket.AuthorID)
		}
	case model.StepTypeUser:
		for _, code := range step.UserCodes {
			id, err := store.UserIDByCode(ctx, tx, code)
			if err == nil {
				add(id)
			}
		}
	case model.StepTypeAuthorCustomizable:
		// Nominations are persisted by pf14BuildRoute (submission payload) or
		// by an earlier draft save, so both sources are read from the store.
		custom, err := store.EngCustomApprovers(ctx, tx, ticket.ID, step.Code)
		if err != nil {
			return nil, err
		}
		for _, id := range custom {
			add(id)
		}
	case model.StepTypeAssigneeCustomizable:
		custom, err := store.EngCustomApprovers(ctx, tx, ticket.ID, step.Code)
		if err != nil {
			return nil, err
		}
		for _, id := range custom {
			add(id)
		}
	case model.StepTypeTeam, model.StepTypeDynamicTeam:
		for _, target := range step.Targets {
			teamIDs, err := pf14TargetTeams(ctx, tx, target, values)
			if err != nil {
				return nil, err
			}
			for _, teamID := range teamIDs {
				members, err := store.EngUsersByTeam(ctx, tx, teamID, target.Descendants)
				if err != nil {
					return nil, err
				}
				gradeTeamIDs := []string{teamID}
				if target.Descendants && target.GradeSymbol != nil {
					gradeTeamIDs, err = pf14bTargetTeamIDs(ctx, tx, teamID, true)
					if err != nil {
						return nil, err
					}
				}
				for _, id := range members {
					matches := false
					for _, gradeTeamID := range gradeTeamIDs {
						matches, err = pf14bUserMatchesGrade(ctx, tx, id, gradeTeamID, target.GradeSymbol, target.GradeCodes)
						if err != nil {
							return nil, err
						}
						if matches {
							break
						}
					}
					if matches {
						add(id)
					}
				}
			}
		}
	case model.StepTypeDynamicUser:
		for _, target := range step.Targets {
			userIDs, err := pf14ResolveUserRefs(ctx, tx, pf14VariableRefs(values, target.Variable))
			if err != nil {
				return nil, err
			}
			for _, id := range userIDs {
				add(id)
			}
		}
	case model.StepTypeManager:
		if ticket.AuthorTeamID != nil {
			ancestors, err := store.EngTeamAncestors(ctx, tx, *ticket.AuthorTeamID)
			if err != nil {
				return nil, err
			}
			for index, team := range ancestors {
				leaders, err := store.EngLeadersOfTeam(ctx, tx, team.ID)
				if err != nil {
					return nil, err
				}
				for _, id := range leaders {
					if ticket.AuthorID != nil && id == *ticket.AuthorID && !step.AllowSelfApproval {
						continue
					}
					matches, er := pf14bUserMatchesTargets(ctx, tx, id, team.ID, step.Targets)
					if er != nil {
						return nil, er
					}
					if matches {
						add(id)
					}
				}
				if len(ids) > 0 && step.ManagerSelectionType != model.ManagerSelectionAll {
					break
				}
				if index == 0 && len(leaders) == 1 && ticket.AuthorID != nil && leaders[0] == *ticket.AuthorID && !step.AllowSelfApproval && step.ManagerSelectionType != model.ManagerSelectionAll {
					continue
				}
			}
		}
	default:
		return nil, fmt.Errorf("engine: unsupported route step type %q", step.StepType)
	}
	if ticket.AuthorID != nil && !step.AllowSelfApproval && step.StepType != model.StepTypeAuthor {
		filtered := ids[:0]
		for _, id := range ids {
			if id != *ticket.AuthorID {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}
	return ids, nil
}

func pf14bTargetTeamIDs(ctx context.Context, q store.Querier, rootID string, descendants bool) ([]string, error) {
	teamIDs := []string{rootID}
	if !descendants {
		return teamIDs, nil
	}
	seen := map[string]bool{rootID: true}
	for index := 0; index < len(teamIDs); index++ {
		children, err := store.GetTeamChildren(ctx, q, teamIDs[index])
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			teamIDs = append(teamIDs, child.ID)
		}
	}
	return teamIDs, nil
}

func pf14bStepConditionMatch(ctx context.Context, tx store.Querier, ticket *model.Ticket, fields []model.ConditionField, combination string, values map[string]any) (bool, error) {
	if len(fields) == 0 {
		return true, nil
	}
	matched := 0
	for _, field := range fields {
		actual, ok := pf14VariableValue(values, field.Variable)
		switch field.FieldKey {
		case model.StepConditionFieldKeyAuthorTeam:
			if ticket.AuthorTeamID == nil {
				ok = false
				break
			}
			actual, ok = *ticket.AuthorTeamID, true
			if field.Symbol == "descendants_or_equal" || field.Symbol == "not_descendants_or_equal" {
				ancestors, err := store.EngTeamAncestors(ctx, tx, *ticket.AuthorTeamID)
				if err != nil && err != store.ErrNotFound {
					return false, err
				}
				found := false
				for _, ancestor := range ancestors {
					if ancestor.ID == fmt.Sprint(field.Value) {
						found = true
						break
					}
				}
				value := found
				if field.Symbol == "not_descendants_or_equal" {
					value = !value
				}
				if value {
					matched++
				}
				continue
			}
		case model.StepConditionFieldKeyAuthorGrade:
			if ticket.AuthorID == nil || ticket.AuthorTeamID == nil {
				ok = false
				break
			}
			memberships, err := store.EngActiveMemberships(ctx, tx, *ticket.AuthorID, *ticket.AuthorTeamID)
			if err != nil {
				return false, err
			}
			gradeID := fmt.Sprint(field.Value)
			if byCode, err := store.GradeIDByCode(ctx, tx, gradeID); err == nil {
				gradeID = byCode
			}
			want, err := store.GetGrade(ctx, tx, gradeID)
			if err != nil {
				ok = false
				break
			}
			for _, membership := range memberships {
				for _, id := range membership.GradeIDs {
					grade, er := store.GetGrade(ctx, tx, id)
					if er != nil {
						return false, er
					}
					if pf14Compare(grade.Level, field.Symbol, want.Level) {
						matched++
						ok = false
						break
					}
				}
				if !ok {
					break
				}
			}
			continue
		case model.StepConditionFieldKeyTextVariable, model.StepConditionFieldKeyNumberVariable, model.StepConditionFieldKeyCheckboxVariable, model.StepConditionFieldKeyGeneralMasterVariable:
			actual, ok = pf14VariableValue(values, field.Variable)
		}
		operator := field.Symbol
		if field.FieldKey == model.StepConditionFieldKeyCheckboxVariable && operator == "equal" {
			operator = "include"
		}
		if ok && pf14Compare(actual, operator, field.Value) {
			matched++
		}
	}
	if combination == model.StepCombinationAny {
		return matched > 0, nil
	}
	return matched == len(fields), nil
}

func pf14bUserMatchesTargets(ctx context.Context, tx *store.Tx, userID, teamID string, targets []model.RouteTarget) (bool, error) {
	for _, target := range targets {
		matches, err := pf14bUserMatchesGrade(ctx, tx, userID, teamID, target.GradeSymbol, target.GradeCodes)
		if err != nil || matches {
			return matches, err
		}
	}
	return len(targets) == 0, nil
}

func pf14bUserMatchesGrade(ctx context.Context, tx store.Querier, userID, teamID string, symbol *string, gradeCodes []string) (bool, error) {
	if symbol == nil || *symbol == "" {
		return true, nil
	}
	if len(gradeCodes) == 0 {
		return false, nil
	}
	memberships, err := store.EngActiveMemberships(ctx, tx, userID, teamID)
	if err != nil {
		return false, err
	}
	gradeIDs := make([]string, 0, len(gradeCodes))
	for _, code := range gradeCodes {
		id, er := store.GradeIDByCode(ctx, tx, code)
		if er != nil {
			continue
		}
		gradeIDs = append(gradeIDs, id)
	}
	wanted := []model.Grade{}
	for _, id := range gradeIDs {
		grade, er := store.GetGrade(ctx, tx, id)
		if er != nil {
			return false, er
		}
		wanted = append(wanted, grade)
	}
	for _, membership := range memberships {
		for _, memberGradeID := range membership.GradeIDs {
			memberGrade, er := store.GetGrade(ctx, tx, memberGradeID)
			if er != nil {
				return false, er
			}
			for _, targetGrade := range wanted {
				match := false
				switch *symbol {
				case model.GradeSymbolEqual:
					match = memberGrade.Level == targetGrade.Level
				case model.GradeSymbolGreaterThan:
					match = memberGrade.Level > targetGrade.Level
				case model.GradeSymbolGreaterThanOrEqual:
					match = memberGrade.Level >= targetGrade.Level
				case model.GradeSymbolLessThan:
					match = memberGrade.Level < targetGrade.Level
				case model.GradeSymbolLessThanOrEqual:
					match = memberGrade.Level <= targetGrade.Level
				case model.GradeSymbolAnyOf:
					match = memberGrade.ID == targetGrade.ID
				}
				if match {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// pf14bUserActive reports whether the user is activated; fallback assignment
// never picks invited, suspended or deactivated users (for example a leader
// who was deleted moments ago).
func pf14bUserActive(ctx context.Context, q store.Querier, userID string) (bool, error) {
	user, err := store.GetUser(ctx, q, userID)
	if err != nil {
		return false, err
	}
	return user.Status == model.UserStatusActive, nil
}

func pf14bFallbackAssignees(ctx context.Context, tx *store.Tx, ticket *model.Ticket, step model.RouteStep, values map[string]any) ([]string, error) {
	if step.FallbackType == nil {
		return nil, nil
	}
	if *step.FallbackType == model.FallbackDirectManager || *step.FallbackType == model.FallbackHigherManager {
		if ticket.AuthorTeamID == nil {
			return nil, nil
		}
		ancestors, err := store.EngTeamAncestors(ctx, tx, *ticket.AuthorTeamID)
		if err != nil {
			return nil, err
		}
		for _, team := range ancestors {
			leaders, err := store.EngLeadersOfTeam(ctx, tx, team.ID)
			if err != nil {
				return nil, err
			}
			ids := []string{}
			for _, id := range leaders {
				if !step.AllowSelfApproval && ticket.AuthorID != nil && id == *ticket.AuthorID {
					continue
				}
				if active, er := pf14bUserActive(ctx, tx, id); er != nil {
					return nil, er
				} else if !active {
					continue
				}
				matches, er := pf14bUserMatchesTargets(ctx, tx, id, team.ID, step.Targets)
				if er != nil {
					return nil, er
				}
				if matches {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				return ids, nil
			}
		}
		return nil, nil
	}
	if *step.FallbackType != model.FallbackHigherTeam {
		return nil, nil
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, target := range step.Targets {
		teamIDs, err := pf14TargetTeams(ctx, tx, target, values)
		if err != nil {
			return nil, err
		}
		if target.TeamCode == "" && target.Variable == "" && ticket.AuthorTeamID != nil {
			teamIDs = []string{*ticket.AuthorTeamID}
		}
		for _, teamID := range teamIDs {
			team, err := store.GetTeam(ctx, tx, teamID)
			if err != nil || team.ParentID == nil {
				continue
			}
			members, err := store.EngUsersByTeam(ctx, tx, *team.ParentID, false)
			if err != nil {
				return nil, err
			}
			for _, id := range members {
				if !step.AllowSelfApproval && ticket.AuthorID != nil && id == *ticket.AuthorID {
					continue
				}
				if active, er := pf14bUserActive(ctx, tx, id); er != nil {
					return nil, er
				} else if !active {
					continue
				}
				matches, er := pf14bUserMatchesGrade(ctx, tx, id, *team.ParentID, target.GradeSymbol, target.GradeCodes)
				if er != nil {
					return nil, er
				}
				if matches && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	}
	return ids, nil
}

func pf14Contains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
