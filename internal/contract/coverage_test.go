package contract_test

import (
	"sort"
	"testing"

	apiv1 "punchflow/internal/api/v1"
)

func TestContractOperationCoverage(t *testing.T) {
	var scenarioIDs []string
	for _, scenario := range liveReadScenarios {
		scenarioIDs = append(scenarioIDs, scenario.operationID)
	}
	scenarioIDs = append(scenarioIDs,
		liveTicketsOperations...,
	)
	scenarioIDs = append(scenarioIDs, liveCommentsFilesOperations...)
	scenarioIDs = append(scenarioIDs, liveUsersGradesOperations...)
	scenarioIDs = append(scenarioIDs, liveOrgChartsTeamsOperations...)
	scenarioIDs = append(scenarioIDs, liveMastersOperations...)
	scenarioIDs = append(scenarioIDs, liveProxiesRolesOperations...)

	seen := make(map[string]struct{}, len(scenarioIDs))
	duplicates := make([]string, 0)
	for _, operationID := range scenarioIDs {
		if _, exists := seen[operationID]; exists {
			duplicates = append(duplicates, operationID)
			continue
		}
		seen[operationID] = struct{}{}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		t.Errorf("live scenarios contain duplicate operationIds: %v", duplicates)
	}
	if len(scenarioIDs) != 96 {
		t.Errorf("live scenario inventory has %d operationIds, want 96", len(scenarioIDs))
	}

	registered := make(map[string]struct{})
	for _, operation := range apiv1.Operations() {
		registered[operation.OperationID] = struct{}{}
	}

	missing := make([]string, 0)
	for operationID := range registered {
		if _, exists := seen[operationID]; !exists {
			missing = append(missing, operationID)
		}
	}
	extra := make([]string, 0)
	for operationID := range seen {
		if _, exists := registered[operationID]; !exists {
			extra = append(extra, operationID)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("live scenarios are missing registered operationIds: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("live scenarios contain unregistered operationIds: %v", extra)
	}
}
