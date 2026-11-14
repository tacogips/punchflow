package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

// decodeObject decodes a successful JSON object response.
func decodeObject(t *testing.T, res *testutil.Response) map[string]any {
	t.Helper()
	if res.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var out map[string]any
	res.JSON(t, &out)
	return out
}

func TestProxyPatchNullClearsDateBounds(t *testing.T) {
	for _, kind := range []string{"proxyApplicants", "proxyApprovers"} {
		t.Run(kind, func(t *testing.T) {
			env := testutil.New(t)
			created := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/"+kind, map[string]any{
				"userId": env.UserID(t, "sales-rep"), "proxyUserId": env.UserID(t, "engineer"),
				"startsOn": "2026-01-01", "endsOn": "2026-12-31",
			}))
			id := created["id"].(string)
			if created["startsOn"] != "2026-01-01" || created["endsOn"] != "2026-12-31" {
				t.Fatalf("created bounds = %v/%v", created["startsOn"], created["endsOn"])
			}

			// Absent properties leave both bounds untouched.
			untouched := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/"+kind+"/"+id, map[string]any{"workflowIds": []string{}}))
			if untouched["startsOn"] != "2026-01-01" || untouched["endsOn"] != "2026-12-31" {
				t.Fatalf("absent bounds changed: %v/%v", untouched["startsOn"], untouched["endsOn"])
			}

			// An explicit null clears only that bound.
			cleared := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/"+kind+"/"+id, map[string]any{"endsOn": nil}))
			if cleared["endsOn"] != nil || cleared["startsOn"] != "2026-01-01" {
				t.Fatalf("after endsOn:null bounds = %v/%v, want 2026-01-01/null", cleared["startsOn"], cleared["endsOn"])
			}
			both := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/"+kind+"/"+id, map[string]any{"startsOn": nil}))
			if both["startsOn"] != nil || both["endsOn"] != nil {
				t.Fatalf("after startsOn:null bounds = %v/%v, want null/null", both["startsOn"], both["endsOn"])
			}
		})
	}
}

func TestGeneralMasterItemPatchNullClearsDateBounds(t *testing.T) {
	env := testutil.New(t)
	path := "/v1/generalMasters/cost-centers/items/cc-legacy"
	before := decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, path, nil))
	if before["endsOn"] != "2020-12-31" {
		t.Fatalf("fixture endsOn = %v", before["endsOn"])
	}
	renamed := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{"name": "Legacy renamed"}))
	if renamed["endsOn"] != "2020-12-31" || renamed["name"] != "Legacy renamed" {
		t.Fatalf("absent endsOn changed: %v", renamed)
	}
	cleared := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{"endsOn": nil}))
	if cleared["endsOn"] != nil {
		t.Fatalf("endsOn after null = %v, want null", cleared["endsOn"])
	}
	again := decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, path, nil))
	if again["endsOn"] != nil {
		t.Fatalf("stored endsOn after null = %v, want null", again["endsOn"])
	}
}

func masterField(t *testing.T, master map[string]any, code string) map[string]any {
	t.Helper()
	fields, _ := master["fields"].([]any)
	for _, raw := range fields {
		field := raw.(map[string]any)
		if field["code"] == code {
			return field
		}
	}
	t.Fatalf("master has no field %q: %v", code, master["fields"])
	return nil
}

func TestGeneralMasterPatchFieldsMergesByCode(t *testing.T) {
	env := testutil.New(t)
	path := "/v1/generalMasters/cost-centers"
	before := masterField(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, path, nil)), "budget-owner")

	// Only code is required: title alone must neither fail nor reset other settings.
	patched := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{
		"fields": []any{map[string]any{"code": "budget-owner", "title": "Owner of budget"}},
	}))
	field := masterField(t, patched, "budget-owner")
	if field["title"] != "Owner of budget" {
		t.Fatalf("title = %v", field["title"])
	}
	for _, key := range []string{"id", "fieldType", "required", "visible", "initialDisplay"} {
		if field[key] != before[key] {
			t.Errorf("field %s changed from %v to %v although it was omitted", key, before[key], field[key])
		}
	}
	if before["initialDisplay"] != true {
		t.Fatalf("fixture precondition: initialDisplay=%v want true", before["initialDisplay"])
	}

	// Explicitly supplied settings are applied, others stay.
	patched = decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{
		"fields": []any{map[string]any{"code": "budget-owner", "required": true, "visible": false}},
	}))
	field = masterField(t, patched, "budget-owner")
	if field["required"] != true || field["visible"] != false || field["title"] != "Owner of budget" || field["initialDisplay"] != true {
		t.Fatalf("explicit merge result = %v", field)
	}

	// A new code needs title and fieldType; existing fields cannot change type.
	if got := env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{"fields": []any{map[string]any{"code": "fresh"}}}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("incomplete new field status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{"fields": []any{map[string]any{"code": "budget-owner", "fieldType": "integer"}}}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("fieldType change status=%d body=%s", got.Status, got.Body)
	}
	added := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, path, map[string]any{
		"fields": []any{
			map[string]any{"code": "budget-owner"},
			map[string]any{"code": "fresh", "title": "Fresh", "fieldType": "integer"},
		},
	}))
	if masterField(t, added, "fresh")["required"] != false || masterField(t, added, "budget-owner")["id"] != before["id"] {
		t.Fatalf("add result = %v", added["fields"])
	}
}
