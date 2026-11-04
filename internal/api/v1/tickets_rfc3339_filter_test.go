package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestListTicketsAcceptsRFC3339MicrosecondFilter(t *testing.T) {
	env := testutil.New(t)
	for _, timestamp := range []string{
		"2026-10-28T04:08:57.101584%2B00:00",
		"2026-10-28T04:08:57.101Z",
	} {
		t.Run(timestamp, func(t *testing.T) {
			r := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?updatedAtStart="+timestamp, nil)
			if r.Status != http.StatusOK {
				t.Fatalf("list tickets: status = %d, body = %s", r.Status, r.Body)
			}
			var rows []map[string]any
			r.JSON(t, &rows)
			if rows == nil {
				t.Fatal("list tickets returned null, want JSON array")
			}
		})
	}
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?updatedAtStart=not-a-time", nil), http.StatusBadRequest, "invalid_parameter")
}
