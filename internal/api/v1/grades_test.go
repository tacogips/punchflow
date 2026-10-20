package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestGradesListAndPermissions(t *testing.T) {
	env := testutil.New(t)
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/grades?perPage=2", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	var rows []map[string]any
	list.JSON(t, &rows)
	if len(rows) != 2 {
		t.Fatalf("grade count=%d", len(rows))
	}
	testutil.AssertPagination(t, list, 1, 2, 5)
	if denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/grades", nil); denied.Status != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", denied.Status, denied.Body)
	}
}

func TestGradeCreateUpdateAndConstraints(t *testing.T) {
	env := testutil.New(t)
	tooHigh := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"name": "Too High", "level": 300})
	testutil.AssertError(t, tooHigh, http.StatusBadRequest, "invalid_parameter")
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"name": "Temporary", "code": "temporary", "level": 200})
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var grade map[string]any
	created.JSON(t, &grade)
	id := grade["id"].(string)
	duplicateLevel := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"name": "Duplicate", "level": 200})
	testutil.AssertError(t, duplicateLevel, http.StatusUnprocessableEntity, "validation_failed")
	byCode := env.Do(t, testutil.Admin, http.MethodGet, "/v1/grades/temporary", nil)
	if byCode.Status != http.StatusOK {
		t.Fatalf("get by code status=%d body=%s", byCode.Status, byCode.Body)
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/grades/"+id, map[string]any{"name": "Temporary Updated", "level": 201})
	if updated.Status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Status, updated.Body)
	}
	var changed map[string]any
	updated.JSON(t, &changed)
	if changed["name"] != "Temporary Updated" || changed["level"] != float64(201) {
		t.Fatalf("updated grade=%v", changed)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/grades/"+id, nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("delete status=%d body=%s", deleted.Status, deleted.Body)
	}
	var audits int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='grade.create'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("grade.create audit count=%d err=%v", audits, err)
	}
}

func TestGradeDefaultAndDeleteGuards(t *testing.T) {
	env := testutil.New(t)
	staff := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/grades/staff", nil)
	testutil.AssertError(t, staff, http.StatusUnprocessableEntity, "validation_failed")
	unused := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades", map[string]any{"name": "Unused", "code": "unused", "level": 222})
	if unused.Status != http.StatusOK {
		t.Fatalf("create unused grade status=%d body=%s", unused.Status, unused.Body)
	}
	var unusedGrade map[string]any
	unused.JSON(t, &unusedGrade)
	setUnusedDefault := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades/unused/default", nil)
	if setUnusedDefault.Status != http.StatusOK {
		t.Fatalf("set unused default status=%d body=%s", setUnusedDefault.Status, setUnusedDefault.Body)
	}
	defaultDelete := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/grades/unused", nil)
	testutil.AssertError(t, defaultDelete, http.StatusUnprocessableEntity, "validation_failed")
	set := env.Do(t, testutil.Admin, http.MethodPost, "/v1/grades/manager/default", nil)
	if set.Status != http.StatusOK || len(set.Body) != 0 {
		t.Fatalf("set default status=%d body=%s", set.Status, set.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/grades/unused", nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("unused grade delete status=%d body=%s", deleted.Status, deleted.Body)
	}
	var managerDefault, previousDefault int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT is_default FROM grades WHERE code='manager'`).Scan(&managerDefault); err != nil {
		t.Fatal(err)
	}
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT is_default FROM grades WHERE code='staff'`).Scan(&previousDefault); err != nil {
		t.Fatal(err)
	}
	if managerDefault != 1 || previousDefault != 0 {
		t.Fatalf("default flags manager=%d staff=%d", managerDefault, previousDefault)
	}
	var audits int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='grade.setDefault'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("grade.setDefault audit count=%d err=%v", audits, err)
	}
}
