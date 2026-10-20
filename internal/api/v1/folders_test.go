package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestFolderCRUDAndRootGuard(t *testing.T) {
	env := testutil.New(t)
	list := env.Do(t, testutil.Employee, http.MethodGet, "/v1/folders?perPage=2", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	var rows []map[string]any
	list.JSON(t, &rows)
	if len(rows) != 2 {
		t.Fatalf("folders length=%d", len(rows))
	}
	testutil.AssertPagination(t, list, 1, 2, 3)
	root := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/folders/root", map[string]any{"name": "Changed"})
	if root.Status != http.StatusForbidden {
		t.Fatalf("root update=%d %s", root.Status, root.Body)
	}
	used := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/finance", nil)
	if used.Status != http.StatusUnprocessableEntity {
		t.Fatalf("used delete=%d %s", used.Status, used.Body)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{"name": "Temporary", "parentFolderId": "general"})
	if created.Status != http.StatusOK {
		t.Fatalf("create=%d %s", created.Status, created.Body)
	}
	var item map[string]any
	created.JSON(t, &item)
	id := item["id"].(string)
	if item["fullName"] != "Root/General/Temporary" {
		t.Fatalf("fullName=%v", item["fullName"])
	}
	got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/folders/"+id, nil)
	if got.Status != http.StatusOK {
		t.Fatalf("get=%d %s", got.Status, got.Body)
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/folders/"+id, map[string]any{"name": "Renamed"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update=%d %s", updated.Status, updated.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete=%d %s", deleted.Status, deleted.Body)
	}
}
