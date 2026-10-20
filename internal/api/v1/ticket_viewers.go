package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listTicketLinks(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	ticketID := r.PathValue("ticketId")
	if err = a.d.Engine.CanView(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	ids, total, err := store.TicketQueryLinks(r.Context(), a.d.DB.Q(), ticketID, page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, err := present.Tickets(r.Context(), a.d.DB.Q(), ids)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) listViewers(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	sort := httpx.ParseSort(r, "createdAt")
	if raw := r.URL.Query().Get("sortBy"); raw != "" && sort.Field == "" {
		kferr.Write(w, kferr.InvalidParameter("Invalid sortBy"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	ticketID := r.PathValue("ticketId")
	if err = a.d.Engine.CanView(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	ids, total, err := store.TicketQueryViewers(r.Context(), a.d.DB.Q(), ticketID, page.Offset(), page.PerPage, sort.Desc)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	all, err := present.TicketViewers(r.Context(), a.d.DB.Q(), []string{ticketID})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	byID := make(map[string]present.TicketViewerJSON, len(all))
	for _, viewer := range all {
		byID[viewer["id"].(string)] = viewer
	}
	out := make([]present.TicketViewerJSON, 0, len(ids))
	for _, id := range ids {
		if viewer, ok := byID[id]; ok {
			out = append(out, viewer)
		}
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}

type pf22ViewerBody struct {
	UserID      json.RawMessage `json:"userId"`
	TeamID      json.RawMessage `json:"teamId"`
	GradeID     *string         `json:"gradeId"`
	Descendants *bool           `json:"descendants"`
}

func (a *API) createViewer(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	var body pf22ViewerBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	users, userSet, err := pf22ViewerIDs(body.UserID)
	if err != nil {
		kferr.Write(w, kferr.InvalidParameter("Invalid userId"))
		return
	}
	teams, teamSet, err := pf22ViewerIDs(body.TeamID)
	if err != nil {
		kferr.Write(w, kferr.InvalidParameter("Invalid teamId"))
		return
	}
	if userSet == teamSet || ((body.GradeID != nil || body.Descendants != nil) && !teamSet) {
		kferr.Write(w, kferr.InvalidParameter("Specify exactly one of userId or teamId; gradeId and descendants require teamId"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	ticketID := r.PathValue("ticketId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	if err = a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	ticket, err := store.Ticket(r.Context(), a.d.DB.Q(), ticketID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	definition, err := store.WorkflowDefByID(r.Context(), a.d.DB.Q(), ticket.WorkflowID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	if !definition.AllowEditingOfViewers && !p.Has(model.PermissionTicketWrite) {
		kferr.Write(w, kferr.MissingPermission("Missing permission"))
		return
	}
	resolvedUsers := []string{}
	for _, id := range users {
		resolved, e := store.ResolveID(r.Context(), a.d.DB.Q(), "users", id)
		if e != nil {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"userId": {"does not exist"}}))
			return
		}
		resolvedUsers = append(resolvedUsers, resolved)
	}
	resolvedTeams := []string{}
	for _, id := range teams {
		resolved, e := store.ResolveID(r.Context(), a.d.DB.Q(), "teams", id)
		if e != nil {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"teamId": {"does not exist"}}))
			return
		}
		resolvedTeams = append(resolvedTeams, resolved)
	}
	var grade *string
	if body.GradeID != nil {
		id, e := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", *body.GradeID)
		if e != nil {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"gradeId": {"does not exist"}}))
			return
		}
		grade = &id
	}
	descendants := body.Descendants != nil && *body.Descendants
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		for _, id := range resolvedUsers {
			if e := store.TicketQueryInsertViewer(r.Context(), tx, model.TicketViewer{ID: store.NewID(), TicketID: ticketID, UserID: &id}); e != nil {
				return e
			}
		}
		for _, id := range resolvedTeams {
			if e := store.TicketQueryInsertViewer(r.Context(), tx, model.TicketViewer{ID: store.NewID(), TicketID: ticketID, TeamID: &id, GradeID: grade, Descendants: descendants}); e != nil {
				return e
			}
		}
		return store.InsertAuditLog(r.Context(), tx, pf22Audit(p.User.ID, "ticketViewer.create", ticketID, r))
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) deleteViewer(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	ticketID, viewerID := r.PathValue("ticketId"), r.PathValue("viewerId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, ticketID); err != nil {
		kferr.Write(w, err)
		return
	}
	ticket, err := store.Ticket(r.Context(), a.d.DB.Q(), ticketID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	definition, err := store.WorkflowDefByID(r.Context(), a.d.DB.Q(), ticket.WorkflowID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	if !definition.AllowEditingOfViewers && !p.Has(model.PermissionTicketWrite) {
		kferr.Write(w, kferr.MissingPermission("Missing permission"))
		return
	}
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.TicketQueryDeleteViewer(r.Context(), tx, ticketID, viewerID); e != nil {
			if e == store.ErrNotFound {
				return kferr.ResourceNotFound()
			}
			return e
		}
		return store.InsertAuditLog(r.Context(), tx, pf22Audit(p.User.ID, "ticketViewer.delete", ticketID, r))
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
func pf22ViewerIDs(raw json.RawMessage) ([]string, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if one == "" {
			return nil, false, nil
		}
		if !store.IsUUID(one) {
			return nil, false, errViewerID
		}
		return []string{one}, true, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil || len(many) == 0 {
		return nil, false, errViewerID
	}
	for _, id := range many {
		if !store.IsUUID(id) {
			return nil, false, errViewerID
		}
	}
	return many, true, nil
}

var errViewerID = &pf22ViewerIDError{}

type pf22ViewerIDError struct{}

func (*pf22ViewerIDError) Error() string { return "invalid viewer identifier" }
