package v1

import (
	"net/http"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionAudit); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
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
	desc := !sort.Set || sort.Desc
	items, total, err := store.AuditLogList(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage, userID, start, end, desc)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make([]present.AuditLogJSON, 0, len(items))
	for _, item := range items {
		itemJSON, err := present.AuditLog(r.Context(), a.d.DB.Q(), item)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		out = append(out, itemJSON)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}

var _ *time.Time
