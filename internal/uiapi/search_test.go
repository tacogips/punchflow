package uiapi_test

import (
	"net/http"
	"net/url"
	"sort"
	"testing"

	"punchflow/internal/testutil"
)

type searchHit struct {
	ID           string  `json:"id"`
	Title        *string `json:"title"`
	TicketNumber *string `json:"ticketNumber"`
}

func (e *expenseEnv) search(t *testing.T, persona testutil.Persona, query url.Values) ([]searchHit, *testutil.Response) {
	t.Helper()
	response := e.Do(t, persona, http.MethodGet, "/ui/api/tickets/search?"+query.Encode(), nil)
	if response.Status != http.StatusOK {
		t.Fatalf("search %v: status=%d body=%s", query, response.Status, response.Body)
	}
	var hits []searchHit
	response.JSON(t, &hits)
	return hits, response
}

func ids(hits []searchHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.ID)
	}
	sort.Strings(out)
	return out
}

func want(values ...string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func assertHits(t *testing.T, label string, hits []searchHit, expected ...string) {
	t.Helper()
	got, wantIDs := ids(hits), want(expected...)
	if len(got) != len(wantIDs) {
		t.Fatalf("%s: hits = %v, want %v", label, got, wantIDs)
	}
	for i := range got {
		if got[i] != wantIDs[i] {
			t.Fatalf("%s: hits = %v, want %v", label, got, wantIDs)
		}
	}
}

func q(keyword string) url.Values { return url.Values{"q": {keyword}} }

func TestKeywordSearchSources(t *testing.T) {
	e := newExpenseEnv(t)
	a := e.createTicket(t, testutil.Employee, ticketSpec{title: "Quarterly offsite", purpose: "team building lunch", rows: []rowSpec{{item: "Bento Box", qty: 3, unit: 1200}}})
	b := e.createTicket(t, testutil.Employee, ticketSpec{title: "Taxi fare", purpose: "airport transfer"})
	comment := e.post(t, testutil.Manager, "/v1/tickets/"+b+"/comments", map[string]any{"body": "Please attach the original Receipt"})
	if comment.Status != http.StatusOK {
		t.Fatalf("comment: %d %s", comment.Status, comment.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	comment.JSON(t, &created)

	var number string
	{
		response := e.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+a, nil)
		var detail searchHit
		response.JSON(t, &detail)
		if detail.TicketNumber == nil || *detail.TicketNumber == "" {
			t.Fatalf("ticket has no number: %s", response.Body)
		}
		number = *detail.TicketNumber
	}

	cases := []struct {
		name    string
		keyword string
		want    []string
	}{
		{"title", "offsite", want(a)},
		{"title is case-insensitive", "OFFSITE", want(a)},
		{"form input value", "building", want(a)},
		{"detail row cell", "bento", want(a)},
		{"comment body", "receipt", want(b)},
		{"ticket number", number, want(a)},
		{"every term must match", "offsite lunch", want(a)},
		{"terms across sources", "taxi receipt", want(b)},
		{"full-width space separates terms", "offsite　lunch", want(a)},
		{"missing term excludes", "offsite airport", nil},
		{"unknown word", "zzzz-not-there", nil},
		{"percent is literal", "%", nil},
		{"underscore is literal", "t_xi", nil},
		{"blank keyword lists everything visible", "   ", want(a, b)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits, _ := e.search(t, testutil.Employee, q(tc.keyword))
			assertHits(t, tc.name, hits, tc.want...)
		})
	}

	t.Run("deleted comments are not searchable", func(t *testing.T) {
		response := e.Do(t, testutil.Manager, http.MethodDelete, "/v1/tickets/"+b+"/comments/"+created.ID, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("delete comment: %d %s", response.Status, response.Body)
		}
		hits, _ := e.search(t, testutil.Employee, q("receipt"))
		assertHits(t, "after delete", hits)
	})

	t.Run("structured filters combine with the keyword", func(t *testing.T) {
		hits, _ := e.search(t, testutil.Employee, url.Values{"q": {"offsite"}, "status[]": {"draft"}})
		assertHits(t, "wrong status", hits)
		hits, _ = e.search(t, testutil.Employee, url.Values{"q": {"offsite"}, "status[]": {"in_progress"}})
		assertHits(t, "right status", hits, a)
	})

	t.Run("invalid filters are reported as /v1 reports them", func(t *testing.T) {
		response := e.Do(t, testutil.Employee, http.MethodGet, "/ui/api/tickets/search?"+url.Values{"q": {"x"}, "status[]": {"bogus"}}.Encode(), nil)
		testutil.AssertError(t, response, http.StatusBadRequest, "invalid_parameter")
	})

	t.Run("pagination covers the keyword result", func(t *testing.T) {
		_, response := e.search(t, testutil.Employee, url.Values{"q": {"e"}, "perPage": {"1"}})
		testutil.AssertPagination(t, response, 1, 1, 2)
		second, _ := e.search(t, testutil.Employee, url.Values{"q": {"e"}, "perPage": {"1"}, "page": {"2"}})
		if len(second) != 1 {
			t.Fatalf("second page hits = %d, want 1", len(second))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, "", http.MethodGet, "/ui/api/tickets/search?q=x", nil), http.StatusUnauthorized, "invalid_access_token")
	})
}

func TestKeywordSearchRespectsTicketVisibility(t *testing.T) {
	e := newExpenseEnv(t)
	draft := e.createTicket(t, testutil.Employee, ticketSpec{status: "draft", title: "Confidential draft", purpose: "secret"})

	hits, _ := e.search(t, testutil.Employee, q("confidential"))
	assertHits(t, "author", hits, draft)
	hits, _ = e.search(t, testutil.Admin, q("confidential"))
	assertHits(t, "ticket_read administrator", hits, draft)
	hits, _ = e.search(t, testutil.Finance, q("confidential"))
	assertHits(t, "user without access", hits)
	hits, _ = e.search(t, testutil.Finance, q("secret"))
	assertHits(t, "user without access, form value", hits)
}

func TestKeywordSearchRespectsSectionViewRestrictions(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Restricted details", purpose: "confidential purchase"})

	// Restrict the first form section to ticket_read administrators and approvers.
	e.setWorkflowDefinition(t, "$.sectionList[0].viewRestriction", `true`)
	e.setWorkflowDefinition(t, "$.sectionList[0].allowViewAuthor", `false`)
	e.setWorkflowDefinition(t, "$.sectionList[0].allowViewApprover", `true`)

	hits, _ := e.search(t, testutil.Employee, q("confidential"))
	assertHits(t, "author cannot match a restricted value", hits)
	hits, _ = e.search(t, testutil.Employee, q("restricted"))
	assertHits(t, "author still matches the title", hits, id)
	hits, _ = e.search(t, testutil.Manager, q("confidential"))
	assertHits(t, "approver cannot match a restricted value", hits)
	hits, _ = e.search(t, testutil.Admin, q("confidential"))
	assertHits(t, "administrator cannot match a restricted value", hits)
	hits, _ = e.search(t, testutil.Employee, q("restricted confidential"))
	assertHits(t, "a restricted term cannot be satisfied by title alone", hits)
}
