package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"punchflow/internal/fixtures"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func workflowCandidate(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../fixtures/default/workflows.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []map[string]any
	if err = json.Unmarshal(b, &defs); err != nil {
		t.Fatal(err)
	}
	var replace func(any)
	replace = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if k == "id" {
					if _, ok := child.(string); ok {
						x[k] = store.NewID()
						continue
					}
				}
				replace(child)
			}
		case []any:
			for _, child := range x {
				replace(child)
			}
		}
	}
	def := defs[0]
	replace(def)
	def["code"] = "argus-asset-registration"
	def["name"] = "Asset registration"
	return def
}
func fixtureFirstID(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../fixtures/default/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &defs); err != nil {
		t.Fatal(err)
	}
	return defs[0].ID
}
func workflowCounts(t *testing.T, env *testutil.Env) (int, int) {
	t.Helper()
	var w, a int
	if err := env.DB.Q().QueryRowContext(t.Context(), "SELECT count(*) FROM workflows").Scan(&w); err != nil {
		t.Fatal(err)
	}
	if err := env.DB.Q().QueryRowContext(t.Context(), "SELECT count(*) FROM audit_logs WHERE action='workflow.create'").Scan(&a); err != nil {
		t.Fatal(err)
	}
	return w, a
}
func field(def map[string]any, section int, key string) map[string]any {
	return def["sectionList"].([]any)[section].(map[string]any)[key].([]any)[0].(map[string]any)
}
func customRule(combination, codeKey, code string) map[string]any {
	return map[string]any{"id": store.NewID(), "combinationType": combination, "errorMessage": "invalid", "fields": []any{map[string]any{"id": store.NewID(), codeKey: code, "symbol": "is_empty"}}}
}

func TestCreateWorkflowSuccess(t *testing.T) {
	env := testutil.New(t)
	def := workflowCandidate(t)
	id := def["id"].(string)
	r := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/workflows", def)
	if r.Status != http.StatusCreated {
		t.Fatalf("create: %d %s", r.Status, r.Body)
	}
	var body map[string]any
	r.JSON(t, &body)
	if body["id"] != id {
		t.Fatalf("id: %v", body)
	}
	list := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows", nil)
	if list.Status != 200 || !bytes.Contains(list.Body, []byte(def["code"].(string))) {
		t.Fatalf("list: %d %s", list.Status, list.Body)
	}
	detail := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/"+id, nil)
	if detail.Status != 200 {
		t.Fatalf("detail: %d %s", detail.Status, detail.Body)
	}
	team, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	fields := def["sectionList"].([]any)[0].(map[string]any)["formFields"].([]any)
	inputs := []any{}
	values := map[string]any{"amount": "100", "purpose": "purpose", "expense-date": "2026-01-01", "approver-note": ""}
	for _, v := range fields {
		f := v.(map[string]any)
		if f["code"] == "cost-center" {
			inputs = append(inputs, map[string]any{"formFieldId": f["id"], "generalMasterItemId": "00000000-0000-4000-8000-000000000041"})
		}
		if f["code"] == "receipt" {
			inputs = append(inputs, map[string]any{"formFieldId": f["id"], "files": []string{}})
		}
		if value, ok := values[f["code"].(string)]; ok {
			inputs = append(inputs, map[string]any{"formFieldId": f["id"], "value": value})
		}
	}
	ticket := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{"status": "draft", "workflowId": id, "authorTeamId": team, "inputs": inputs})
	if ticket.Status != 200 {
		t.Fatalf("ticket: %d %s", ticket.Status, ticket.Body)
	}
	_, a := workflowCounts(t, env)
	if a != 1 {
		t.Fatalf("audit count %d", a)
	}
}
func TestCreateWorkflowGeneratedID(t *testing.T) {
	env := testutil.New(t)
	def := workflowCandidate(t)
	delete(def, "id")
	r := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/workflows", def)
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Body)
	}
	var result struct {
		ID string `json:"id"`
	}
	r.JSON(t, &result)
	if len(result.ID) != 36 || result.ID[14] != '4' {
		t.Fatalf("generated UUID v4: %q", result.ID)
	}
}

func TestCreateWorkflowRejections(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		code, message string
		mutate        func(map[string]any)
		persona       testutil.Persona
		malformed     bool
	}{
		{name: "unauthorized", status: 401, code: "invalid_access_token"},
		{name: "employee", status: 403, code: "missing_permission", persona: testutil.Employee},
		{name: "malformed", status: 400, code: "invalid_json", persona: testutil.Admin, malformed: true},
		{name: "unknown field", status: 400, code: "invalid_parameter", persona: testutil.Admin, mutate: func(d map[string]any) { d["bogus"] = 1 }},
		{name: "unknown route", status: 400, code: "invalid_parameter", message: "routeCode", persona: testutil.Admin, mutate: func(d map[string]any) { d["routeCode"] = "no-such-route" }},
		{name: "unknown folder", status: 400, code: "invalid_parameter", message: "folderCode", persona: testutil.Admin, mutate: func(d map[string]any) { d["folderCode"] = "no-such-folder" }},
		{name: "form validation", status: 400, code: "invalid_parameter", message: "combinationType", persona: testutil.Admin, mutate: func(d map[string]any) {
			f := field(d, 0, "formFields")
			f["customValidations"] = []any{customRule("bogus", "formFieldCode", f["code"].(string))}
		}},
		{name: "slip validation", status: 400, code: "invalid_parameter", message: "combinationType", persona: testutil.Admin, mutate: func(d map[string]any) {
			f := field(d, 1, "slipFields")
			f["customValidations"] = []any{customRule("bogus", "slipFieldCode", f["code"].(string))}
		}},
		{name: "validation limit", status: 400, code: "invalid_parameter", message: "limit of 50 form validations", persona: testutil.Admin, mutate: func(d map[string]any) {
			f := field(d, 0, "formFields")
			rules := []any{}
			for i := 0; i < 51; i++ {
				rules = append(rules, customRule("all", "formFieldCode", f["code"].(string)))
			}
			f["customValidations"] = rules
		}},
		{name: "duplicate code", status: 409, code: "conflict", persona: testutil.Admin, mutate: func(d map[string]any) { d["code"] = "expense-claim" }},
		{name: "duplicate workflow id", status: 409, code: "conflict", persona: testutil.Admin, mutate: func(d map[string]any) { d["id"] = fixtureFirstID(t, "workflows") }},
		{name: "duplicate token id", status: 400, code: "invalid_parameter", message: "tokens.json", persona: testutil.Admin, mutate: func(d map[string]any) { d["id"] = fixtureFirstID(t, "tokens") }},
		{name: "duplicate user id", status: 400, code: "invalid_parameter", message: "duplicate id", persona: testutil.Admin, mutate: func(d map[string]any) { d["id"] = fixtureFirstID(t, "users") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.New(t)
			beforeW, beforeA := workflowCounts(t, env)
			def := workflowCandidate(t)
			if tc.mutate != nil {
				tc.mutate(def)
			}
			var r *testutil.Response
			if tc.malformed {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/api/workflows", strings.NewReader("{"))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer pf_pat_dev_admin")
				r = env.DoRaw(t, req)
			} else {
				r = env.Do(t, tc.persona, http.MethodPost, "/admin/api/workflows", def)
			}
			testutil.AssertError(t, r, tc.status, tc.code)
			if tc.message != "" && !bytes.Contains(r.Body, []byte(tc.message)) {
				t.Fatalf("missing %q: %s", tc.message, r.Body)
			}
			if tc.name == "duplicate token id" && !bytes.Contains(r.Body, []byte("duplicate id")) {
				t.Fatalf("missing duplicate id: %s", r.Body)
			}
			if strings.Contains(tc.name, "validation") && tc.name != "validation limit" && !bytes.Contains(r.Body, []byte("workflows[")) {
				t.Fatalf("missing validation path: %s", r.Body)
			}
			afterW, afterA := workflowCounts(t, env)
			if beforeW != afterW || beforeA != afterA {
				t.Fatalf("rejection changed counts: %d/%d -> %d/%d", beforeW, beforeA, afterW, afterA)
			}
		})
	}
}
func TestCreateWorkflowIgnoresBaselineValidationProblems(t *testing.T) {
	env := testutil.New(t)
	_, err := env.DB.Q().ExecContext(t.Context(), "INSERT INTO webhooks(id,name,url,secret,enabled,workflow_ids,event_types,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", store.NewID(), "bad", "https://example.test", "secret", true, `["`+store.NewID()+`"]`, `[]`, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	files, err := fixtures.Export(t.Context(), env.DB.Q())
	if err != nil {
		t.Fatal(err)
	}
	fs := fstest.MapFS{}
	for n, b := range files {
		fs[n] = &fstest.MapFile{Data: b}
	}
	set, err := fixtures.Parse(fs)
	if err != nil {
		t.Fatal(err)
	}
	err = fixtures.Validate(set)
	if err == nil || !strings.Contains(err.Error(), "webhooks.json") {
		t.Fatalf("baseline validation: %v", err)
	}
	r := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/workflows", workflowCandidate(t))
	if r.Status != 201 {
		t.Fatalf("create with baseline problem: %d %s", r.Status, r.Body)
	}
}

func TestCreateWorkflowFolderScopeDenied(t *testing.T) {
	env := testutil.New(t)
	_, err := env.DB.Q().ExecContext(t.Context(), `UPDATE roles SET permission_list = '[{"permission":"workflow","restricted":true,"folderIds":[]}]' WHERE id = ?`, "00000000-0000-4000-8000-000000000029")
	if err != nil {
		t.Fatal(err)
	}
	beforeW, beforeA := workflowCounts(t, env)
	r := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/workflows", workflowCandidate(t))
	testutil.AssertError(t, r, http.StatusForbidden, "missing_permission")
	afterW, afterA := workflowCounts(t, env)
	if beforeW != afterW || beforeA != afterA {
		t.Fatalf("denial changed counts: %d/%d -> %d/%d", beforeW, beforeA, afterW, afterA)
	}
}
