package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestFixturesExportAndResetGate(t *testing.T) {
	env := testutil.New(t)
	exported := env.Do(t, testutil.Admin, http.MethodGet, "/admin/api/fixtures/export", nil)
	if exported.Status != http.StatusOK {
		t.Fatalf("export: %d %s", exported.Status, exported.Body)
	}
	var files map[string]json.RawMessage
	exported.JSON(t, &files)
	if len(files) != 15 {
		t.Fatalf("export has %d fixture files, want 15", len(files))
	}
	var masters []struct {
		Items []struct {
			Inputs []struct {
				ID string `json:"id"`
			} `json:"inputs"`
		} `json:"items"`
	}
	if err := json.Unmarshal(files["general-masters.json"], &masters); err != nil {
		t.Fatal(err)
	}
	for _, master := range masters {
		for _, item := range master.Items {
			for _, input := range item.Inputs {
				if input.ID == "" {
					t.Fatal("exported general master input lacks id")
				}
			}
		}
	}
	denied := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/fixtures/reset", map[string]any{})
	testutil.AssertError(t, denied, http.StatusNotFound, "endpoint_not_found")
}
func TestFixturesResetEnabled(t *testing.T) {
	env := testutil.New(t, testutil.WithAdminReset())
	reset := env.Do(t, testutil.Admin, http.MethodPost, "/admin/api/fixtures/reset", map[string]any{})
	if reset.Status != http.StatusOK {
		t.Fatalf("reset: %d %s", reset.Status, reset.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/admin/api/tenant", nil); got.Status != http.StatusOK {
		t.Fatalf("seeded token unusable after reset: %d %s", got.Status, got.Body)
	}
}
