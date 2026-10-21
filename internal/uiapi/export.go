package uiapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"punchflow/internal/access"
	"punchflow/internal/auth"
	"punchflow/internal/export"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// searchTickets lists the tickets matching the /v1 ticket filters plus an
// optional free-text keyword (q), as the same JSON array GET /v1/tickets
// returns, paginated with the same headers.
func (a *API) searchTickets(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	items, ok := a.collectTickets(w, r)
	if !ok {
		return
	}
	start, end := page.Offset(), page.Offset()+page.PerPage
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	out := make([]json.RawMessage, 0, end-start)
	for _, item := range items[start:end] {
		out = append(out, item.Raw)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, len(items))
	httpx.WriteJSON(w, http.StatusOK, out)
}

// exportTickets downloads the complete filtered, authorised and keyword-matched
// ticket result as CSV in the ticket (default) or detail layout.
func (a *API) exportTickets(w http.ResponseWriter, r *http.Request) {
	layout := r.URL.Query().Get("layout")
	if layout == "" {
		layout = export.LayoutTicket
	}
	if layout != export.LayoutTicket && layout != export.LayoutDetail {
		kferr.Write(w, kferr.InvalidParameter("Invalid layout"))
		return
	}
	items, ok := a.collectTickets(w, r)
	if !ok {
		return
	}
	p, _ := auth.FromContext(r.Context())
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	caller := access.Caller{UserID: p.User.ID, TicketRead: p.Permissions[model.PermissionTicketRead]}
	var body bytes.Buffer
	if err := export.WriteTickets(r.Context(), a.d.DB.Q(), &body, layout, ids, caller); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	a.writeCSV(w, "tickets-"+layout, body.Bytes())
}

// exportAuditLogs downloads every audit log matching the list filters as CSV.
func (a *API) exportAuditLogs(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionAudit); err != nil {
		kferr.Write(w, err)
		return
	}
	start, err := httpx.QueryTime(r, "createdAtStart")
	if err != nil {
		kferr.Write(w, err)
		return
	}
	end, err := httpx.QueryTime(r, "createdAtEnd")
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if start != nil && end != nil && start.After(*end) {
		kferr.Write(w, kferr.InvalidParameter("createdAtStart must not be after createdAtEnd"))
		return
	}
	userID := r.URL.Query().Get("userId")
	if userID != "" && !store.IsUUID(userID) {
		kferr.Write(w, kferr.InvalidParameter("Invalid userId"))
		return
	}
	sort := httpx.ParseSort(r, "createdAt")
	var body bytes.Buffer
	filter := export.AuditFilter{UserID: userID, Start: start, End: end, Desc: !sort.Set || sort.Desc}
	if err = export.WriteAuditLogs(r.Context(), a.d.DB.Q(), &body, filter); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	a.writeCSV(w, "audit-logs", body.Bytes())
}

func (a *API) writeCSV(w http.ResponseWriter, name string, body []byte) {
	filename := name + "-" + a.d.Clock.Now().In(kftime.JST).Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", export.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
