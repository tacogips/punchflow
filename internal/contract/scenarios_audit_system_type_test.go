package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestContractMutationAuditSystemTypeIsNull(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	cache := &pf26eSchemaCache{compiler: schemaCompiler(t, spec), compiled: make(map[string]*jsonschema.Schema)}
	chart, err := store.ActiveOrgChartID(t.Context(), env.DB.Q())
	if err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+chart+"/teams", map[string]any{"name": "Audit contract team", "code": "audit-contract"})
	pf26eCheck(t, "createTeam", created, spec, cache)
	response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/auditLogs?perPage=100", nil)
	pf26eCheck(t, "listAuditLogs", response, spec, cache)
	var logs []struct {
		Action     string  `json:"action"`
		SystemType *string `json:"systemType"`
	}
	if err := json.Unmarshal(response.Body, &logs); err != nil {
		t.Fatal(err)
	}
	for _, log := range logs {
		if log.Action == "team.create" {
			if log.SystemType != nil {
				t.Fatalf("team.create systemType=%q, want null", *log.SystemType)
			}
			return
		}
	}
	t.Fatal("audit list omitted mutation-generated team.create record")
}
