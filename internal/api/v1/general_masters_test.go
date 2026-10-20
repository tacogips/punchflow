package v1_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"punchflow/internal/testutil"
)

func TestGeneralMasterPermissionsAndValidation(t *testing.T) {
	env := testutil.New(t)
	if denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters", nil); denied.Status != http.StatusForbidden {
		t.Fatalf("employee list=%d %s", denied.Status, denied.Body)
	}
	env.CallerFor = "accountant"
	list := env.Do(t, testutil.Integration, http.MethodGet, "/v1/generalMasters", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("master editor list=%d %s", list.Status, list.Body)
	}
	invalid := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Invalid sort", "defaultSortBy": "createdAt"})
	if invalid.Status != http.StatusBadRequest {
		t.Fatalf("invalid sort=%d %s", invalid.Status, invalid.Body)
	}
	duplicate := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Duplicate fields", "fields": []any{map[string]any{"code": "x", "title": "X", "required": false, "fieldType": "text"}, map[string]any{"code": "x", "title": "Again", "required": false, "fieldType": "text"}}})
	if duplicate.Status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate fields=%d %s", duplicate.Status, duplicate.Body)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{
		"name":   "Temporary master",
		"fields": []any{map[string]any{"code": "country", "title": "Country", "required": false, "fieldType": "pull_down", "options": []string{"JP", "US"}}},
	})
	if created.Status != http.StatusOK {
		t.Fatalf("create master=%d %s", created.Status, created.Body)
	}
	var master map[string]any
	created.JSON(t, &master)
	id := master["id"].(string)
	if fetched := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/"+id, nil); fetched.Status != http.StatusOK {
		t.Fatalf("get master=%d %s", fetched.Status, fetched.Body)
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/"+id, map[string]any{"name": "Updated master"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update master=%d %s", updated.Status, updated.Body)
	}
	item := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+id+"/items", map[string]any{"name": "Temporary item", "inputs": []any{map[string]any{"code": "country", "value": "JP"}}})
	if item.Status != http.StatusOK {
		t.Fatalf("create master item=%d %s", item.Status, item.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/generalMasters/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete master=%d %s", deleted.Status, deleted.Body)
	}
	var remaining int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM general_master_items WHERE general_master_id=?`, id).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("deleted master retained %d items", remaining)
	}
	if missing := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/not-a-master", nil); missing.Status != http.StatusNotFound {
		t.Fatalf("unknown master=%d %s", missing.Status, missing.Body)
	}
}

func TestGeneralMasterItemsAndInputIDHop(t *testing.T) {
	env := testutil.New(t)
	if denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/generalMasters", nil); denied.Status != http.StatusForbidden {
		t.Fatalf("employee list=%d", denied.Status)
	}
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/cost-centers/items", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("items=%d %s", list.Status, list.Body)
	}
	var items []map[string]any
	list.JSON(t, &items)
	if len(items) != 3 {
		t.Fatalf("items count=%d", len(items))
	}
	if items[0]["code"] != "cc-eng" {
		t.Fatalf("default sort item=%v", items[0]["code"])
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/cost-centers/items", map[string]any{"name": "New center", "inputs": []any{map[string]any{"code": "budget-owner", "value": "new-owner"}}})
	if created.Status != http.StatusOK {
		t.Fatalf("create item=%d %s", created.Status, created.Body)
	}
	var item map[string]any
	created.JSON(t, &item)
	inputs := item["inputs"].([]any)
	input := inputs[0].(map[string]any)
	inputID := input["id"].(string)
	fieldID := input["field"].(map[string]any)["id"].(string)
	if inputID == fieldID {
		t.Fatalf("input id reuses field id %s", inputID)
	}
	var persisted string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT input_ids FROM general_master_items WHERE id=?`, item["id"]).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	var idMap map[string]string
	if err := json.Unmarshal([]byte(persisted), &idMap); err != nil {
		t.Fatal(err)
	}
	if idMap[fieldID] != inputID {
		t.Fatalf("stored id map=%v, response input id=%s", idMap, inputID)
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/generalMasters/cost-centers/items/"+item["id"].(string), map[string]any{"inputs": []any{map[string]any{"code": "budget-owner", "value": "updated-owner"}}})
	if updated.Status != http.StatusOK {
		t.Fatalf("update item=%d %s", updated.Status, updated.Body)
	}
	var after map[string]any
	updated.JSON(t, &after)
	afterInput := after["inputs"].([]any)[0].(map[string]any)
	if afterInput["id"] != inputID || afterInput["value"] != "updated-owner" {
		t.Fatalf("updated input=%v want stable id %s", afterInput, inputID)
	}
	fixtureBytes, err := os.ReadFile("../../../fixtures/default/general-masters.json")
	if err != nil {
		t.Fatal(err)
	}
	var masters []struct {
		Code  string `json:"code"`
		Items []struct {
			Code   string `json:"code"`
			Inputs []struct {
				ID        string `json:"id"`
				FieldCode string `json:"fieldCode"`
			} `json:"inputs"`
		} `json:"items"`
	}
	if err = json.Unmarshal(fixtureBytes, &masters); err != nil {
		t.Fatal(err)
	}
	var fixtureID string
	for _, m := range masters {
		if m.Code == "cost-centers" {
			for _, it := range m.Items {
				if it.Code == "cc-sales" && len(it.Inputs) > 0 {
					fixtureID = it.Inputs[0].ID
				}
			}
		}
	}
	if fixtureID == "" {
		t.Fatal("cc-sales fixture input ID not found")
	}
	got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/generalMasters/cost-centers/items/cc-sales", nil)
	if got.Status != http.StatusOK {
		t.Fatalf("get fixture item=%d %s", got.Status, got.Body)
	}
	var sales map[string]any
	got.JSON(t, &sales)
	if sales["inputs"].([]any)[0].(map[string]any)["id"] != fixtureID {
		t.Fatalf("fixture input ID=%v want %s", sales["inputs"].([]any)[0].(map[string]any)["id"], fixtureID)
	}
	invalid := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/cost-centers/items", map[string]any{"name": "Bad", "inputs": []any{map[string]any{"code": "missing", "value": "x"}}})
	if invalid.Status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field=%d %s", invalid.Status, invalid.Body)
	}
	invalidOption := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/vendors/items", map[string]any{"name": "Bad country", "inputs": []any{map[string]any{"code": "country", "value": "CA"}}})
	if invalidOption.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid pull_down option=%d %s", invalidOption.Status, invalidOption.Body)
	}
}
