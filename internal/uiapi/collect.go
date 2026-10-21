package uiapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"punchflow/internal/access"
	"punchflow/internal/auth"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/ticketsearch"
)

const (
	collectPerPage = 100
	// maxCollected bounds the tickets a search or export gathers in memory.
	maxCollected = 10000
)

// ownParams are consumed here and never forwarded to the /v1 ticket list.
var ownParams = map[string]bool{"q": true, "page": true, "perPage": true, "layout": true}

// ticketItem is one ticket of a collected result, kept as the /v1 list JSON so
// search responses are shape-identical to GET /v1/tickets.
type ticketItem struct {
	ID  string
	Raw json.RawMessage
}

// responseRecorder captures a handler response without importing httptest.
type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRecorder() *responseRecorder {
	return &responseRecorder{header: http.Header{}, status: http.StatusOK}
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) WriteHeader(status int)      { r.status = status }
func (r *responseRecorder) Write(p []byte) (int, error) { return r.body.Write(p) }

// collectTickets returns every ticket the caller may read that matches the
// request's structured filters (forwarded verbatim to GET /v1/tickets, which
// also applies effective visibility) and, when q is given, the keyword. The
// order is the /v1 list order. A non-nil error has already been written.
func (a *API) collectTickets(w http.ResponseWriter, r *http.Request) ([]ticketItem, bool) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return nil, false
	}
	p, _ := auth.FromContext(r.Context())
	forwarded := url.Values{}
	for key, values := range r.URL.Query() {
		if !ownParams[key] {
			forwarded[key] = values
		}
	}
	forwarded.Set("perPage", strconv.Itoa(collectPerPage))

	var items []ticketItem
	for page := 1; ; page++ {
		forwarded.Set("page", strconv.Itoa(page))
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "/v1/tickets?"+forwarded.Encode(), nil)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return nil, false
		}
		rec := newRecorder()
		a.d.V1.ServeHTTP(rec, req)
		if rec.status != http.StatusOK {
			// Surface the /v1 validation error (for example an invalid filter) as is.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(rec.status)
			_, _ = w.Write(rec.body.Bytes())
			return nil, false
		}
		var raws []json.RawMessage
		if err = json.Unmarshal(rec.body.Bytes(), &raws); err != nil {
			kferr.Write(w, kferr.Internal())
			return nil, false
		}
		for _, raw := range raws {
			var head struct {
				ID string `json:"id"`
			}
			if err = json.Unmarshal(raw, &head); err != nil || head.ID == "" {
				kferr.Write(w, kferr.Internal())
				return nil, false
			}
			items = append(items, ticketItem{ID: head.ID, Raw: raw})
		}
		total, _ := strconv.Atoi(rec.header.Get("Total"))
		if len(raws) == 0 || len(items) >= total {
			break
		}
		if len(items) >= maxCollected {
			kferr.Write(w, kferr.InvalidParameter(fmt.Sprintf("More than %d tickets match; narrow the filters", maxCollected)))
			return nil, false
		}
	}

	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(ticketsearch.Terms(keyword)) == 0 {
		return items, true
	}
	caller := access.Caller{UserID: p.User.ID, TicketRead: p.Permissions[model.PermissionTicketRead]}
	matched, err := ticketsearch.Match(r.Context(), a.d.DB.Q(), caller, keyword)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return nil, false
	}
	filtered := items[:0]
	for _, item := range items {
		if matched[item.ID] {
			filtered = append(filtered, item)
		}
	}
	return filtered, true
}
