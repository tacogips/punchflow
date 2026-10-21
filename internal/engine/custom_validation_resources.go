package engine

import (
	"encoding/json"
	"fmt"

	"punchflow/internal/conditions"
	"punchflow/internal/store"
)

func pf12MatchResourceConditions(combination string, count int, matches func(int) bool) bool {
	if count == 0 {
		return false
	}
	matched := 0
	for i := 0; i < count; i++ {
		if matches(i) {
			matched++
		}
	}
	if combination == "any" {
		return matched > 0
	}
	return matched == count
}

func (n *normalizer) resourceConditionMatches(actual any, symbol string, value *string, masterCode, itemCode, gradeCode, teamCode string) bool {
	if itemCode != "" && masterCode != "" {
		masterID, err := store.ResolveID(n.ctx, n.q, "general_masters", masterCode)
		if err != nil {
			return false
		}
		itemID, err := store.MasterItemIDByCode(n.ctx, n.q, masterID, itemCode)
		if err != nil {
			return false
		}
		return pf12ReferenceCompare(actual, symbol, itemID)
	}
	if gradeCode != "" {
		gradeID, err := store.GradeIDByCode(n.ctx, n.q, gradeCode)
		if err != nil {
			return false
		}
		threshold, err := store.GetGrade(n.ctx, n.q, gradeID)
		if err != nil {
			return false
		}
		matched := false
		for _, id := range pf12ReferenceIDs(actual) {
			if actualGrade, err := store.GetGrade(n.ctx, n.q, id); err == nil {
				matched = matched || conditions.Compare(actualGrade.Level, symbol, threshold.Level)
			} else if n.userGradeMatches(id, threshold.Level, symbol) {
				matched = true
			}
		}
		if symbol == "not_equal" {
			return !matched && len(pf12ReferenceIDs(actual)) > 0
		}
		return matched
	}
	if teamCode != "" {
		chartID, err := store.ActiveOrgChartID(n.ctx, n.q)
		if err != nil {
			return false
		}
		teamID, err := store.TeamIDByCode(n.ctx, n.q, chartID, teamCode)
		if err != nil {
			return false
		}
		matched := false
		for _, id := range pf12ReferenceIDs(actual) {
			if _, err := store.GetTeam(n.ctx, n.q, id); err == nil {
				if symbol == "descendants_or_equal" {
					matched = matched || n.teamDescendsFrom(id, teamID)
				} else {
					matched = matched || id == teamID
				}
			} else if n.userHasTeamRelation(id, teamID, symbol == "descendants_or_equal") {
				matched = true
			}
		}
		if symbol == "not_equal" || symbol == "exclude" {
			return !matched && len(pf12ReferenceIDs(actual)) > 0
		}
		return matched
	}
	var expected any
	if value != nil {
		expected = *value
	}
	return conditions.Compare(actual, symbol, expected)
}

func (n *normalizer) userGradeMatches(userID string, threshold int, symbol string) bool {
	rows, err := n.q.QueryContext(n.ctx, `SELECT m.grade_ids FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE c.current=1 AND m.user_id=?`, userID)
	if err != nil {
		return false
	}
	var membershipGrades [][]string
	checkSymbol := symbol
	if symbol == "not_equal" {
		checkSymbol = "equal"
	}
	found := false
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return false
		}
		var gradeIDs []string
		if json.Unmarshal([]byte(raw), &gradeIDs) != nil {
			_ = rows.Close()
			return false
		}
		membershipGrades = append(membershipGrades, gradeIDs)
	}
	if rows.Err() != nil {
		_ = rows.Close()
		return false
	}
	if rows.Close() != nil {
		return false
	}
	for _, gradeIDs := range membershipGrades {
		for _, gradeID := range gradeIDs {
			grade, err := store.GetGrade(n.ctx, n.q, gradeID)
			if err == nil && conditions.Compare(grade.Level, checkSymbol, threshold) {
				if symbol == "not_equal" {
					return false
				}
				found = true
			}
		}
	}
	if symbol == "not_equal" {
		return !found
	}
	return found
}

func (n *normalizer) userHasTeamRelation(userID, expectedTeamID string, descendants bool) bool {
	rows, err := n.q.QueryContext(n.ctx, `SELECT m.team_id FROM team_memberships m JOIN teams t ON t.id=m.team_id JOIN organization_charts c ON c.id=t.organization_chart_id WHERE c.current=1 AND m.user_id=?`, userID)
	if err != nil {
		return false
	}
	var teamIDs []string
	for rows.Next() {
		var teamID string
		if err := rows.Scan(&teamID); err != nil {
			_ = rows.Close()
			return false
		}
		teamIDs = append(teamIDs, teamID)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return false
	}
	for _, teamID := range teamIDs {
		if (descendants && n.teamDescendsFrom(teamID, expectedTeamID)) || (!descendants && teamID == expectedTeamID) {
			return true
		}
	}
	return false
}

func (n *normalizer) teamDescendsFrom(teamID, ancestorID string) bool {
	seen := map[string]bool{}
	for teamID != "" && !seen[teamID] {
		if teamID == ancestorID {
			return true
		}
		seen[teamID] = true
		team, err := store.GetTeam(n.ctx, n.q, teamID)
		if err != nil || team.ParentID == nil {
			return false
		}
		teamID = *team.ParentID
	}
	return false
}

func pf12ReferenceCompare(actual any, symbol, expectedID string) bool {
	found := false
	for _, id := range pf12ReferenceIDs(actual) {
		if id == expectedID {
			found = true
			break
		}
	}
	switch symbol {
	case "not_equal", "exclude":
		return !found
	case "is_empty":
		return len(pf12ReferenceIDs(actual)) == 0
	case "is_not_empty":
		return len(pf12ReferenceIDs(actual)) > 0
	case "equal", "include", "descendants_or_equal":
		return found
	default:
		return conditions.Compare(actual, symbol, expectedID)
	}
}

func pf12ReferenceIDs(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			if id, ok := item.(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	case fmt.Stringer:
		return []string{v.String()}
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(v, &decoded) == nil {
			return pf12ReferenceIDs(decoded)
		}
	case []byte:
		var decoded any
		if json.Unmarshal(v, &decoded) == nil {
			return pf12ReferenceIDs(decoded)
		}
	}
	return nil
}
