package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestCategoriesCRUDAndPermissions(t *testing.T) {
	env := testutil.New(t)
	list := env.Do(t, testutil.Employee, http.MethodGet, "/v1/categories?perPage=1", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	var rows []map[string]any
	list.JSON(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("list length=%d", len(rows))
	}
	testutil.AssertPagination(t, list, 1, 1, 3)
	if denied := env.Do(t, testutil.Employee, http.MethodPost, "/v1/categories", map[string]any{"name": "New"}); denied.Status != http.StatusForbidden {
		t.Fatalf("create status=%d body=%s", denied.Status, denied.Body)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/categories", map[string]any{"name": "Temporary"})
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var item map[string]any
	created.JSON(t, &item)
	id := item["id"].(string)
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/categories/"+id, map[string]any{"name": "Temp updated"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Status, updated.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/categories/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Status, deleted.Body)
	}
	used := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/categories/expense", nil)
	if used.Status != http.StatusUnprocessableEntity {
		t.Fatalf("used delete status=%d body=%s", used.Status, used.Body)
	}
}
