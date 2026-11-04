package contract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestContractRouteConditionIncludesMasterMetadata(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	routeID, err := store.ResolveID(t.Context(), env.DB.Q(), "routes", "expense-standard")
	if err != nil {
		t.Fatal(err)
	}
	route, err := store.RouteDefByID(t.Context(), env.DB.Q(), routeID)
	if err != nil {
		t.Fatal(err)
	}
	route.Steps[0].RouteStepCondition = &model.RouteStepCondition{
		ID: "00000000-0000-4000-8000-000000000077", ConditionType: "conditional", CombinationType: "all",
		RouteStepConditionFields: []model.ConditionField{{ID: "00000000-0000-4000-8000-000000000078", FieldKey: "general_master_variable", Symbol: "equal", GeneralMasterCode: "cost-centers", GeneralMasterItemCode: "cc-eng"}},
	}
	raw, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE routes SET definition_json=? WHERE id=?`, string(raw), routeID); err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/routes/"+routeID, nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get route: %d %s", response.Status, response.Body)
	}
	var operation apiv1.Operation
	for _, candidate := range apiv1.Operations() {
		if candidate.OperationID == "getRoute" {
			operation = candidate
			break
		}
	}
	if operation.OperationID == "" {
		t.Fatal("getRoute is not registered")
	}
	schema, err := compiler.Compile(responseSchemaURI(operation.Pattern, "get"))
	if err != nil {
		t.Fatal(err)
	}
	validateJSON(t, schema, response.Body)
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	steps := body["steps"].([]any)
	condition := steps[0].(map[string]any)["routeStepCondition"].(map[string]any)
	field := condition["routeStepConditionFields"].([]any)[0].(map[string]any)
	master, ok := field["generalMaster"].(map[string]any)
	if !ok {
		t.Fatalf("condition field missing resolved generalMaster: %v", field)
	}
	item, ok := field["generalMasterItem"].(map[string]any)
	if !ok {
		t.Fatalf("condition field missing resolved generalMasterItem: %v", field)
	}
	if master["code"] != "cost-centers" || master["itemsCount"] != float64(3) || master["initialDisplayDescription"] != true {
		t.Fatalf("generalMaster metadata is incomplete: %v", master)
	}
	if item["code"] != "cc-eng" || field["value"] != nil {
		t.Fatalf("master item condition metadata/value = %v", field)
	}
}
