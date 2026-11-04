package v1_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

func TestINFV9API01GradeNullCodeDoesNotMutateGrade(t *testing.T) {
	env := testutil.New(t)
	before := env.Do(t, testutil.Admin, http.MethodGet, "/v1/grades/manager", nil)
	if before.Status != http.StatusOK {
		t.Fatalf("get grade=%d %s", before.Status, before.Body)
	}
	var grade map[string]any
	before.JSON(t, &grade)
	got := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/grades/"+grade["id"].(string), map[string]any{"code": nil})
	testutil.AssertError(t, got, http.StatusBadRequest, "invalid_parameter")
	var code string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT code FROM grades WHERE id=?`, grade["id"]).Scan(&code); err != nil || code != "manager" {
		t.Fatalf("grade code=%q err=%v, want unchanged manager", code, err)
	}
}

func TestINFV9API02GeneralMasterPlanAndTextLimits(t *testing.T) {
	env := testutil.New(t)
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tenant SET plan='standard'`); err != nil {
		t.Fatal(err)
	}
	fields := []any{}
	for i := 0; i < 10; i++ {
		fields = append(fields, map[string]any{"code": "f" + string(rune('a'+i)), "title": "Field", "required": false, "fieldType": "text"})
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Ten fields", "fields": fields})
	if created.Status != http.StatusOK {
		t.Fatalf("standard plan 10-field master=%d %s", created.Status, created.Body)
	}
	tooMany := append(fields, map[string]any{"code": "extra", "title": "Extra", "required": false, "fieldType": "text"})
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Eleven fields", "fields": tooMany}); got.Status != http.StatusForbidden {
		t.Fatalf("standard plan 11-field master=%d %s, want upgrade_required", got.Status, got.Body)
	}
	masterResponse := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Text boundaries", "fields": []any{
		map[string]any{"code": "short", "title": "Short", "required": false, "fieldType": "text"},
		map[string]any{"code": "long", "title": "Long", "required": false, "fieldType": "text_long"},
	}})
	if masterResponse.Status != http.StatusOK {
		t.Fatalf("create text master=%d %s", masterResponse.Status, masterResponse.Body)
	}
	var textMaster map[string]any
	masterResponse.JSON(t, &textMaster)
	masterID := textMaster["id"].(string)
	for _, tc := range []struct {
		field  string
		length int
		want   int
	}{
		{"short", 300, http.StatusOK},
		{"short", 301, http.StatusUnprocessableEntity},
		{"long", 10000, http.StatusOK},
		{"long", 10001, http.StatusUnprocessableEntity},
	} {
		shortValue, longValue := "ok", "ok"
		if tc.field == "short" {
			shortValue = strings.Repeat("界", tc.length)
		} else {
			longValue = strings.Repeat("界", tc.length)
		}
		got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+masterID+"/items", map[string]any{
			"name": "Text value", "inputs": []any{
				map[string]any{"code": "short", "value": shortValue},
				map[string]any{"code": "long", "value": longValue},
			},
		})
		if got.Status != tc.want {
			t.Fatalf("short text length %d status=%d body=%s, want %d", tc.length, got.Status, got.Body, tc.want)
		}
	}
	var masterCount int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM general_masters`).Scan(&masterCount); err != nil {
		t.Fatal(err)
	}
	if masterCount < 30 {
		toAdd := 30 - masterCount
		if _, err := env.DB.Q().ExecContext(t.Context(), `WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<?) INSERT INTO general_masters(id,code,name,default_sort_by,created_at,updated_at) SELECT printf('r9-master-%02d',n),'r9-master-'||n,'Master','name',0,0 FROM seq`, toAdd+1); err != nil {
			t.Fatal(err)
		}
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Master thirty one"}); got.Status != http.StatusForbidden {
		t.Fatalf("31st standard-plan master status=%d body=%s, want upgrade_required", got.Status, got.Body)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<100000) INSERT INTO general_master_items(id,general_master_id,code,name,inputs,input_ids,created_at,updated_at) SELECT printf('r9-item-%06d',n),?,'r9-item-'||n,'Item','{}','{}',0,0 FROM seq`, masterID); err != nil {
		t.Fatal(err)
	}
	blocked := env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters/"+masterID+"/items", map[string]any{"name": "Over quota", "inputs": []any{
		map[string]any{"code": "short", "value": "ok"}, map[string]any{"code": "long", "value": "ok"},
	}})
	if blocked.Status != http.StatusUnprocessableEntity {
		t.Fatalf("101st-thousand item status=%d body=%s, want 422", blocked.Status, blocked.Body)
	}
}

func TestINFV9API03ProxyBodyRejectsBusinessCodes(t *testing.T) {
	env := testutil.New(t)
	body := map[string]any{"userId": "sales-rep", "proxyUserId": "assistant", "workflowIds": []string{"expense-claim"}}
	got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/proxyApplicants", body)
	testutil.AssertError(t, got, http.StatusBadRequest, "invalid_parameter")
}

func TestINFV9API04BulkMembershipRejectsDuplicateUserIDs(t *testing.T) {
	env := testutil.New(t)
	var roleID string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT id FROM roles WHERE editable=1 LIMIT 1`).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	userID := env.UserID(t, "sales-rep")
	duplicate := []string{userID, userID}
	var roleBefore int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM role_memberships WHERE role_id=?`, roleID).Scan(&roleBefore); err != nil {
		t.Fatal(err)
	}
	role := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles/"+roleID+"/memberships", map[string]any{"userIds": duplicate})
	testutil.AssertError(t, role, http.StatusBadRequest, "invalid_parameter")
	var roleAfter int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM role_memberships WHERE role_id=?`, roleID).Scan(&roleAfter); err != nil || roleAfter != roleBefore {
		t.Fatalf("role membership count changed from %d to %d: %v", roleBefore, roleAfter, err)
	}
	var teamID string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT id FROM teams WHERE organization_chart_id=? AND code='sales'`, "00000000-0000-4000-8000-000000000027").Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	var teamBefore int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM team_memberships WHERE team_id=?`, teamID).Scan(&teamBefore); err != nil {
		t.Fatal(err)
	}
	team := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/organizationCharts/00000000-0000-4000-8000-000000000027/teams/sales/memberships", map[string]any{"userIds": duplicate})
	testutil.AssertError(t, team, http.StatusBadRequest, "invalid_parameter")
	var teamAfter int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM team_memberships WHERE team_id=?`, teamID).Scan(&teamAfter); err != nil || teamAfter != teamBefore {
		t.Fatalf("team membership count changed from %d to %d: %v", teamBefore, teamAfter, err)
	}
}

func TestINFV9API05OrganizationChartNameCountsUnicodeCharacters(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{"name": strings.Repeat("界", 300)})
	if created.Status != http.StatusOK {
		t.Fatalf("300-character name status=%d body=%s", created.Status, created.Body)
	}
	var chart map[string]any
	created.JSON(t, &chart)
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart["id"].(string), map[string]any{"name": strings.Repeat("界", 301)})
	if updated.Status != http.StatusUnprocessableEntity {
		t.Fatalf("301-character name status=%d body=%s, want 422", updated.Status, updated.Body)
	}
}

func TestINFV9API06PageOffsetOverflowReturnsInvalidParameter(t *testing.T) {
	env := testutil.New(t)
	got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/workflows?page=9223372036854775807&perPage=100", nil)
	testutil.AssertError(t, got, http.StatusBadRequest, "invalid_parameter")
}
