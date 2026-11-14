package ticketui_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestManualTicketLinksAreReciprocalAndCanBeRemoved(t *testing.T) {
	env := testutil.New(t)
	first, second := createTicket(t, env), createTicket(t, env)
	linked := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+first+"/links", map[string]string{"linkedTicketId": second})
	if linked.Status != http.StatusNoContent {
		t.Fatalf("link tickets: %d %s", linked.Status, linked.Body)
	}
	assertTicketLinks(t, env, first, second)
	assertTicketLinks(t, env, second, first)

	unlinked := env.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+first+"/links/"+second, nil)
	if unlinked.Status != http.StatusNoContent {
		t.Fatalf("unlink tickets: %d %s", unlinked.Status, unlinked.Body)
	}
	assertTicketLinks(t, env, first)
	assertTicketLinks(t, env, second)
}

func TestManualTicketLinksRequireVisibilityToBothTickets(t *testing.T) {
	env := testutil.New(t)
	visible, hidden := createTicket(t, env), createTicket(t, env)
	financeID, err := store.ResolveID(t.Context(), env.DB.Q(), "users", "finance-manager")
	if err != nil {
		t.Fatal(err)
	}
	financeTeamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "finance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET author_id=?,author_team_id=? WHERE id=?`, financeID, financeTeamID, hidden); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `DELETE FROM ticket_viewers WHERE ticket_id=?`, hidden); err != nil {
		t.Fatal(err)
	}
	// The control setup makes the target ticket private to the finance team.
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+visible+"/links", map[string]string{"linkedTicketId": hidden}), http.StatusForbidden, "invisible_ticket")
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+visible+"/links/"+hidden, nil), http.StatusForbidden, "invisible_ticket")
	assertTicketLinksFor(t, env, testutil.Employee, visible)
	assertTicketLinksFor(t, env, testutil.Admin, hidden)
}

func assertTicketLinks(t *testing.T, env *testutil.Env, ticketID string, linkedIDs ...string) {
	assertTicketLinksFor(t, env, testutil.Employee, ticketID, linkedIDs...)
}

func assertTicketLinksFor(t *testing.T, env *testutil.Env, persona testutil.Persona, ticketID string, linkedIDs ...string) {
	t.Helper()
	response := env.Do(t, persona, http.MethodGet, "/v1/tickets/"+ticketID+"/links", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get ticket links: %d %s", response.Status, response.Body)
	}
	var links []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body, &links); err != nil {
		t.Fatalf("decode ticket links: %v", err)
	}
	if len(links) != len(linkedIDs) {
		t.Fatalf("links for %s = %+v, want IDs %v", ticketID, links, linkedIDs)
	}
	for i, id := range linkedIDs {
		if links[i].ID != id {
			t.Fatalf("links for %s = %+v, want IDs %v", ticketID, links, linkedIDs)
		}
	}
}
