package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestViewerCreateListDelete(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	gradeID, err := store.ResolveID(t.Context(), env.DB.Q(), "grades", "manager")
	if err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/viewers", map[string]any{"teamId": teamID, "gradeId": gradeID, "descendants": true})
	if created.Status != http.StatusOK || len(created.Body) != 0 {
		t.Fatalf("create viewer: %d %s", created.Status, created.Body)
	}
	bad := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/viewers", map[string]any{"userId": env.UserID(t, "sales-manager"), "teamId": teamID})
	testutil.AssertError(t, bad, http.StatusBadRequest, "invalid_parameter")
	listed := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticketID+"/viewers", nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("list viewers: %d %s", listed.Status, listed.Body)
	}
	var rows []map[string]any
	listed.JSON(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("viewers=%v", rows)
	}
	if rows[0]["id"] == nil || rows[0]["team"] == nil || rows[0]["grade"] == nil || rows[0]["descendants"] != true {
		t.Fatalf("viewer fields=%v", rows[0])
	}
	for header, want := range map[string]string{"Page": "1", "Per-Page": "25", "Total": "1"} {
		if got := listed.Header.Get(header); got != want {
			t.Errorf("%s header = %q, want %q", header, got, want)
		}
	}
	viewerID, ok := rows[0]["id"].(string)
	if !ok {
		t.Fatalf("viewer ID=%v", rows[0]["id"])
	}
	deleted := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/tickets/"+ticketID+"/viewers/"+viewerID, nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("delete viewer: %d %s", deleted.Status, deleted.Body)
	}
	missing := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/tickets/"+ticketID+"/viewers/"+viewerID, nil)
	testutil.AssertError(t, missing, http.StatusNotFound, "resource_not_found")
}
