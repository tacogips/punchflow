package engine

import (
	"context"
	"fmt"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (e *Engine) pf14BuildRoute(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, inputs []FieldInput, approvers []ApproverInput, nowTime int64) error {
	values := pf14InputValues(def, inputs)
	routeCode := def.RouteCode
	matchedBranch := false
	for _, condition := range def.RouteConditions {
		if condition.ConditionType == model.RouteConditionTypeFieldOtherwise {
			if !matchedBranch && condition.RouteCode != nil {
				routeCode = *condition.RouteCode
			}
			break
		}
		match := condition.ConditionType == model.RouteConditionTypeAlways || pf14ConditionMatch(condition.ConditionFields, condition.CombinationType, values)
		if !match {
			continue
		}
		matchedBranch = true
		if condition.ErrorMessage != nil && *condition.ErrorMessage != "" {
			return kferr.ValidationFailed(map[string][]string{"base": {*condition.ErrorMessage}})
		}
		if condition.RouteCode != nil {
			routeCode = *condition.RouteCode
			break
		}
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
		skip := step.RouteStepCondition != nil && step.RouteStepCondition.ConditionType != model.StepConditionTypeAlways && !pf14ConditionMatch(step.RouteStepCondition.RouteStepConditionFields, step.RouteStepCondition.CombinationType, values)
		fb := false
		result := ""
		userIDs, er := e.pf14ResolveAssignees(ctx, tx, ticket, step, values, approvers)
		if er != nil {
			return er
		}
		if len(userIDs) == 0 && step.StepType != model.StepTypeAuthorCustomizable && step.StepType != model.StepTypeAssigneeCustomizable {
			if step.FallbackType != nil {
				fb = true
				result = *step.FallbackType
				if *step.FallbackType == model.FallbackSkip {
					skip = true
				} else if *step.FallbackType == model.FallbackNoFallback {
					return kferr.ValidationFailed(map[string][]string{"route": {"step has no assignees"}})
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
	min := 0
	for group := range groups {
		if min == 0 || group < min {
			min = group
		}
	}
	ticket.CurrentStep = min
	for _, s := range groups[min] {
		if s.Status == model.StepStatusSkipped {
			continue
		}
		s.Status = model.StepStatusInProgress
		s.UpdatedAt = store.Time(nowTime)
		if err = store.EngUpdateStep(ctx, tx, s); err != nil {
			return err
		}
	}
	return nil
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
		for _, v := range approvers {
			if v.RouteStepCode == step.Code {
				for _, id := range v.UserIDs {
					add(id)
				}
			}
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
			teamID := ""
			if target.TeamCode != "" {
				chart, err := store.ActiveOrgChartID(ctx, tx)
				if err != nil {
					return nil, err
				}
				teamID, err = store.TeamIDByCode(ctx, tx, chart, target.TeamCode)
				if err != nil {
					continue
				}
			} else if target.Variable != "" {
				teamID = fmt.Sprint(values[target.Variable])
			}
			if teamID != "" {
				members, err := store.EngUsersByTeam(ctx, tx, teamID, target.Descendants)
				if err != nil {
					return nil, err
				}
				for _, id := range members {
					add(id)
				}
			}
		}
	case model.StepTypeDynamicUser:
		for _, target := range step.Targets {
			if v, ok := values[target.Variable]; ok {
				switch val := v.(type) {
				case string:
					add(val)
				case []any:
					for _, x := range val {
						add(fmt.Sprint(x))
					}
				}
			}
		}
	case model.StepTypeManager:
		if ticket.AuthorTeamID != nil {
			ancestors, err := store.EngTeamAncestors(ctx, tx, *ticket.AuthorTeamID)
			if err != nil {
				return nil, err
			}
			for _, team := range ancestors {
				leaders, err := store.EngLeadersOfTeam(ctx, tx, team.ID)
				if err != nil {
					return nil, err
				}
				for _, id := range leaders {
					if id != *ticket.AuthorID {
						add(id)
					}
				}
				if len(ids) > 0 && step.ManagerSelectionType != model.ManagerSelectionAll {
					break
				}
			}
		}
	default:
		return nil, fmt.Errorf("engine: unsupported route step type %q", step.StepType)
	}
	if ticket.AuthorID != nil && !step.AllowSelfApproval {
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

func pf14Contains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
