package uiapi_test

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

const bom = "\xEF\xBB\xBF"

// readCSV checks the Excel-oriented framing (BOM, CRLF, attachment headers) and
// returns the parsed rows including the header.
func readCSV(t *testing.T, response *testutil.Response, filePrefix string) [][]string {
	t.Helper()
	if response.Status != http.StatusOK {
		t.Fatalf("export: status=%d body=%s", response.Status, response.Body)
	}
	if got := response.Header.Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	disposition := response.Header.Get("Content-Disposition")
	if !regexp.MustCompile(`^attachment; filename="` + regexp.QuoteMeta(filePrefix) + `-\d{8}-\d{6}\.csv"$`).MatchString(disposition) {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	if !bytes.HasPrefix(response.Body, []byte(bom)) {
		t.Fatalf("export lacks a UTF-8 byte order mark: %q", response.Body[:8])
	}
	body := response.Body[len(bom):]
	if !bytes.Contains(body, []byte("\r\n")) {
		t.Fatalf("export does not use CRLF line endings")
	}
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse export: %v\n%s", err, body)
	}
	return rows
}

func columnIndex(t *testing.T, header []string, name string) int {
	t.Helper()
	for i, h := range header {
		if h == name {
			return i
		}
	}
	t.Fatalf("column %q not in header %v", name, header)
	return -1
}

func (e *expenseEnv) exportTickets(t *testing.T, persona testutil.Persona, query url.Values) *testutil.Response {
	t.Helper()
	return e.Do(t, persona, http.MethodGet, "/ui/api/tickets/export?"+query.Encode(), nil)
}

func TestExportTicketLayout(t *testing.T) {
	e := newExpenseEnv(t)
	e.createTicket(t, testutil.Employee, ticketSpec{title: "Dinner, with \"client\"", purpose: "客先との会食", amount: 8800})
	e.createTicket(t, testutil.Employee, ticketSpec{title: "Taxi", purpose: "airport transfer", amount: 4200})
	e.createTicket(t, testutil.Employee, ticketSpec{status: "draft", title: "Private draft", purpose: "books", amount: 100})

	rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{}), "tickets-ticket")
	header := rows[0]
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want header plus the employee's three tickets", len(rows))
	}
	for _, name := range []string{"Ticket number", "Title", "Workflow", "Status", "Author", "Author team", "Created at", "Purpose", "Amount"} {
		columnIndex(t, header, name)
	}
	byTitle := map[string][]string{}
	for _, row := range rows[1:] {
		if len(row) != len(header) {
			t.Fatalf("row has %d cells, header has %d", len(row), len(header))
		}
		byTitle[row[columnIndex(t, header, "Title")]] = row
	}
	dinner := byTitle[`Dinner, with "client"`]
	if dinner == nil {
		t.Fatalf("quoted title row missing: %v", byTitle)
	}
	if got := dinner[columnIndex(t, header, "Purpose")]; got != "客先との会食" {
		t.Fatalf("purpose = %q, want the Japanese text", got)
	}
	if got := dinner[columnIndex(t, header, "Amount")]; got != "8800" {
		t.Fatalf("amount = %q", got)
	}
	if got := dinner[columnIndex(t, header, "Status")]; got != "in_progress" {
		t.Fatalf("status = %q", got)
	}
	if got := dinner[columnIndex(t, header, "Workflow")]; got == "" {
		t.Fatalf("workflow name missing")
	}
	if got := dinner[columnIndex(t, header, "Cost center")]; got == "" || got == "cc-sales" {
		t.Fatalf("master reference should render by name, got %q", got)
	}
	if byTitle["Private draft"] == nil {
		t.Fatalf("author export lacks the draft: %v", byTitle)
	}
	if hidden := readCSV(t, e.exportTickets(t, testutil.Finance, url.Values{"q": {"Private"}}), "tickets-ticket"); len(hidden) != 1 {
		t.Fatalf("export leaked a ticket the requester cannot read: %v", hidden)
	}

	t.Run("export honours the keyword and filters", func(t *testing.T) {
		rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{"q": {"airport"}}), "tickets-ticket")
		if len(rows) != 2 || rows[1][columnIndex(t, rows[0], "Title")] != "Taxi" {
			t.Fatalf("keyword export = %v", rows)
		}
		rows = readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{"status[]": {"draft"}}), "tickets-ticket")
		if len(rows) != 2 || rows[1][columnIndex(t, rows[0], "Title")] != "Private draft" {
			t.Fatalf("status filter export = %v, want only the draft", rows)
		}
		rows = readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{"status[]": {"completed"}}), "tickets-ticket")
		if len(rows) != 1 {
			t.Fatalf("status filter export = %v, want only the header", rows)
		}
	})

	t.Run("export covers more than one page of results", func(t *testing.T) {
		rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{"perPage": {"1"}, "page": {"2"}}), "tickets-ticket")
		if len(rows) != 4 {
			t.Fatalf("paging parameters must not truncate an export: %d rows", len(rows))
		}
	})

	t.Run("invalid layout", func(t *testing.T) {
		testutil.AssertError(t, e.exportTickets(t, testutil.Employee, url.Values{"layout": {"wide"}}), http.StatusBadRequest, "invalid_parameter")
	})
	t.Run("invalid filter", func(t *testing.T) {
		testutil.AssertError(t, e.exportTickets(t, testutil.Employee, url.Values{"status[]": {"bogus"}}), http.StatusBadRequest, "invalid_parameter")
	})
	t.Run("requires authentication", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, "", http.MethodGet, "/ui/api/tickets/export", nil), http.StatusUnauthorized, "invalid_access_token")
	})
}

func TestExportDetailLayout(t *testing.T) {
	e := newExpenseEnv(t)
	e.createTicket(t, testutil.Employee, ticketSpec{title: "Two lines", purpose: "lines", rows: []rowSpec{{"Pens", 2, 150}, {"Paper", 5, 400}}})
	e.createTicket(t, testutil.Employee, ticketSpec{title: "No lines", purpose: "none"})

	rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{"layout": {"detail"}}), "tickets-detail")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want header plus the two detail rows", len(rows))
	}
	header := rows[0]
	section := columnIndex(t, header, "Section")
	rowNo := columnIndex(t, header, "Row no.")
	if rows[1][section] != "Expense lines" || rows[1][rowNo] != "1" || rows[2][rowNo] != "2" {
		t.Fatalf("detail rows = %v", rows[1:])
	}
	item := -1
	for i, h := range header {
		if strings.EqualFold(h, "Item") {
			item = i
		}
	}
	if item < 0 || rows[1][item] != "Pens" || rows[2][item] != "Paper" {
		t.Fatalf("detail item cells wrong: header=%v rows=%v", header, rows[1:])
	}
	if rows[1][columnIndex(t, header, "Title")] != "Two lines" {
		t.Fatalf("detail row lost the ticket title: %v", rows[1])
	}
}

func TestExportNeutralisesSpreadsheetFormulas(t *testing.T) {
	e := newExpenseEnv(t)
	e.createTicket(t, testutil.Employee, ticketSpec{title: "=HYPERLINK(\"http://x\")", purpose: "+1+1"})
	rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{}), "tickets-ticket")
	if got := rows[1][columnIndex(t, rows[0], "Title")]; got != `'=HYPERLINK("http://x")` {
		t.Fatalf("formula title exported as %q", got)
	}
	if got := rows[1][columnIndex(t, rows[0], "Purpose")]; got != "'+1+1" {
		t.Fatalf("formula purpose exported as %q", got)
	}
}

func TestExportRespectsSectionViewRestrictions(t *testing.T) {
	e := newExpenseEnv(t)
	e.createTicket(t, testutil.Employee, ticketSpec{title: "Restricted", purpose: "hidden purpose"})
	e.setWorkflowDefinition(t, "$.sectionList[0].viewRestriction", `true`)
	e.setWorkflowDefinition(t, "$.sectionList[0].allowViewAuthor", `false`)

	rows := readCSV(t, e.exportTickets(t, testutil.Employee, url.Values{}), "tickets-ticket")
	if got := rows[1][columnIndex(t, rows[0], "Purpose")]; got != "" {
		t.Fatalf("author exported a restricted value: %q", got)
	}
	rows = readCSV(t, e.exportTickets(t, testutil.Admin, url.Values{}), "tickets-ticket")
	if got := rows[1][columnIndex(t, rows[0], "Purpose")]; got != "hidden purpose" {
		t.Fatalf("ticket_read administrator purpose = %q", got)
	}
}

func TestExportAuditLogs(t *testing.T) {
	e := newExpenseEnv(t)
	e.createTicket(t, testutil.Employee, ticketSpec{title: "Audited", purpose: "audit"})
	employeeID := e.UserID(t, "sales-rep")

	rows := readCSV(t, e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/audit-logs/export", nil), "audit-logs")
	header := rows[0]
	action := columnIndex(t, header, "Action")
	found := false
	for _, row := range rows[1:] {
		if row[action] == "ticket.create" {
			found = true
			if row[columnIndex(t, header, "User email")] != "sales-rep@punchflow.test" {
				t.Fatalf("audit row user email = %q", row[columnIndex(t, header, "User email")])
			}
			if row[columnIndex(t, header, "Ticket ID")] == "" {
				t.Fatalf("audit row lacks the ticket id")
			}
		}
	}
	if !found {
		t.Fatalf("no ticket.create audit row in %v", rows)
	}

	t.Run("user filter", func(t *testing.T) {
		rows := readCSV(t, e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/audit-logs/export?userId="+employeeID, nil), "audit-logs")
		for _, row := range rows[1:] {
			if row[columnIndex(t, rows[0], "User ID")] != employeeID {
				t.Fatalf("filtered export contains another user: %v", row)
			}
		}
		if len(rows) < 2 {
			t.Fatalf("filtered export is empty")
		}
		other := readCSV(t, e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/audit-logs/export?userId="+e.UserID(t, "accountant"), nil), "audit-logs")
		if len(other) != 1 {
			t.Fatalf("export for a user without activity = %v", other)
		}
	})
	t.Run("requires the audit permission", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, testutil.Employee, http.MethodGet, "/ui/api/audit-logs/export", nil), http.StatusForbidden, "missing_permission")
	})
	t.Run("invalid user id", func(t *testing.T) {
		testutil.AssertError(t, e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/audit-logs/export?userId=nope", nil), http.StatusBadRequest, "invalid_parameter")
	})
	t.Run("invalid period", func(t *testing.T) {
		query := "?createdAtStart=2026-02-01T00:00:00%2B09:00&createdAtEnd=2026-01-01T00:00:00%2B09:00"
		testutil.AssertError(t, e.Do(t, testutil.Admin, http.MethodGet, "/ui/api/audit-logs/export"+query, nil), http.StatusBadRequest, "invalid_parameter")
	})
}
