package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14VariableName normalizes a route or condition variable reference. The
// documented syntax is "$field_code"; "${field_code}" and the bare field code
// are accepted as well.
func pf14VariableName(variable string) string {
	name := strings.TrimSpace(variable)
	name = strings.TrimPrefix(name, "$")
	if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		name = strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}")
	}
	return name
}

// pf14VariableValue looks up the form value a variable reference points to.
func pf14VariableValue(values map[string]any, variable string) (any, bool) {
	v, ok := values[pf14VariableName(variable)]
	if !ok {
		v, ok = values[variable]
	}
	return v, ok
}

// pf14VariableRefs returns the non-empty string references held by a variable.
// A single reference and a list of references are both accepted.
func pf14VariableRefs(values map[string]any, variable string) []string {
	v, ok := pf14VariableValue(values, variable)
	if !ok || v == nil {
		return nil
	}
	refs := []string{}
	add := func(item any) {
		if item == nil {
			return
		}
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			refs = append(refs, s)
		}
	}
	switch val := v.(type) {
	case []any:
		for _, item := range val {
			add(item)
		}
	case []string:
		for _, item := range val {
			add(item)
		}
	default:
		add(val)
	}
	return refs
}

// pf14ResolveUserRefs converts user codes (or IDs) to user IDs. References that
// match no user are dropped.
func pf14ResolveUserRefs(ctx context.Context, q store.Querier, refs []string) ([]string, error) {
	ids := []string{}
	for _, ref := range refs {
		id, err := store.ResolveID(ctx, q, "users", ref)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// pf14ResolveTeamRefs converts team codes of the active organization chart (or
// team IDs of that chart) to team IDs. References that match no team are dropped.
func pf14ResolveTeamRefs(ctx context.Context, q store.Querier, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	chart, err := store.ActiveOrgChartID(ctx, q)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, ref := range refs {
		id, err := store.TeamIDByCode(ctx, q, chart, ref)
		if err == nil {
			ids = append(ids, id)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		inChart, err := store.EngTeamInActiveChart(ctx, q, ref)
		if err != nil {
			return nil, err
		}
		if inChart {
			ids = append(ids, ref)
		}
	}
	return ids, nil
}

// pf14TargetTeams resolves the team(s) a route target designates: a fixed
// team code, or the teams held by a "$field_code" variable.
func pf14TargetTeams(ctx context.Context, q store.Querier, target model.RouteTarget, values map[string]any) ([]string, error) {
	switch {
	case target.TeamCode != "":
		return pf14ResolveTeamRefs(ctx, q, []string{target.TeamCode})
	case target.Variable != "":
		return pf14ResolveTeamRefs(ctx, q, pf14VariableRefs(values, target.Variable))
	}
	return nil, nil
}
