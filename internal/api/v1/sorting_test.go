package v1_test

import (
	"net/http"
	"reflect"
	"testing"

	"punchflow/internal/testutil"
)

func chartNames(t *testing.T, res *testutil.Response) []string {
	t.Helper()
	if res.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var rows []map[string]any
	res.JSON(t, &rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row["name"].(string))
	}
	return out
}

func TestOrganizationChartListHonorsSortBy(t *testing.T) {
	env := testutil.New(t)
	// Created through the API, so it is newer than both fixture charts.
	if res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{"name": "Zeta chart"}); res.Status != http.StatusOK {
		t.Fatalf("create chart: %d %s", res.Status, res.Body)
	}
	for _, tc := range []struct {
		sortBy string
		want   []string
	}{
		{"", []string{"2026 Org", "2027 Plan", "Zeta chart"}},
		{"createdAt-asc", []string{"2026 Org", "2027 Plan", "Zeta chart"}},
		{"createdAt-desc", []string{"Zeta chart", "2026 Org", "2027 Plan"}},
		{"name-asc", []string{"2026 Org", "2027 Plan", "Zeta chart"}},
		{"name-desc", []string{"Zeta chart", "2027 Plan", "2026 Org"}},
		{"name", []string{"2026 Org", "2027 Plan", "Zeta chart"}},
		{"bogus-desc", []string{"2026 Org", "2027 Plan", "Zeta chart"}},
	} {
		t.Run(tc.sortBy, func(t *testing.T) {
			path := "/v1/organizationCharts"
			if tc.sortBy != "" {
				path += "?sortBy=" + tc.sortBy
			}
			if got := chartNames(t, env.Do(t, testutil.Admin, http.MethodGet, path, nil)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("GET %s = %v, want %v", path, got, tc.want)
			}
		})
	}
	// Sorting happens before pagination.
	page := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts?sortBy=name-desc&perPage=1&page=2", nil)
	if got := chartNames(t, page); !reflect.DeepEqual(got, []string{"2027 Plan"}) {
		t.Fatalf("page 2 of name-desc = %v, want [2027 Plan]", got)
	}
}

func viewerUserIDs(t *testing.T, res *testutil.Response) []string {
	t.Helper()
	if res.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var rows []map[string]any
	res.JSON(t, &rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		user, _ := row["user"].(map[string]any)
		out = append(out, user["id"].(string))
	}
	return out
}

func TestTicketViewerListHonorsCreatedAtSort(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	hr, accountant := env.UserID(t, "hr-manager"), env.UserID(t, "accountant")
	// hr-manager is added first although accountant sorts first by e-mail.
	for _, id := range []string{hr, accountant} {
		if res := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/viewers", map[string]any{"userId": id}); res.Status != http.StatusOK {
			t.Fatalf("add viewer: %d %s", res.Status, res.Body)
		}
	}
	for _, tc := range []struct {
		sortBy string
		want   []string
	}{
		{"", []string{accountant, hr}},
		{"createdAt-asc", []string{hr, accountant}},
		{"createdAt", []string{hr, accountant}},
		{"createdAt-desc", []string{accountant, hr}},
		{"email-asc", []string{accountant, hr}},
		{"bogus", []string{accountant, hr}},
	} {
		t.Run(tc.sortBy, func(t *testing.T) {
			path := "/v1/tickets/" + ticketID + "/viewers"
			if tc.sortBy != "" {
				path += "?sortBy=" + tc.sortBy
			}
			if got := viewerUserIDs(t, env.Do(t, testutil.Employee, http.MethodGet, path, nil)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("GET %s = %v, want %v", path, got, tc.want)
			}
		})
	}
}

func TestUnsupportedTicketSortFieldsAreIgnored(t *testing.T) {
	env := testutil.New(t)
	pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	for _, path := range []string{
		"/v1/tickets?sortBy=title-asc",
		"/v1/tickets?sortBy=nonsense",
		"/v1/tickets?sortBy=createdAt-sideways",
		"/v1/tickets?sortBy=openedAt-asc", // tasks-only field on the ticket list
		"/v1/tickets/tasks?sortBy=title-desc",
		"/v1/tickets/tasks?sortBy=a-b-c",
	} {
		t.Run(path, func(t *testing.T) {
			if res := env.Do(t, testutil.Employee, http.MethodGet, path, nil); res.Status != http.StatusOK {
				t.Fatalf("GET %s status=%d, want 200 with default order; body=%s", path, res.Status, res.Body)
			}
		})
	}
}
