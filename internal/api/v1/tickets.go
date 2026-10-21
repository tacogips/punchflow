package v1

import (
	"context"
	"net/http"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listTickets(w http.ResponseWriter, r *http.Request) { a.pf22ListTickets(w, r, false) }
func (a *API) listTasks(w http.ResponseWriter, r *http.Request)   { a.pf22ListTickets(w, r, true) }

func (a *API) pf22ListTickets(w http.ResponseWriter, r *http.Request, tasks bool) {
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
	filter := store.TicketQueryFilter{UserID: p.User.ID, Tasks: tasks}
	q := r.URL.Query()
	if values, e := httpx.QueryArray(r, "status", true); e != nil {
		kferr.Write(w, e)
		return
	} else {
		allowed := model.TicketStatuses
		if tasks {
			allowed = []string{model.TicketStatusDraft, model.TicketStatusInProgress, model.TicketStatusRejected, model.TicketStatusCompleted, "deleted"}
		}
		for _, value := range values {
			valid := false
			for _, candidate := range allowed {
				if value == candidate {
					valid = true
					break
				}
			}
			if !valid {
				kferr.Write(w, kferr.InvalidParameter("Invalid status"))
				return
			}
			if tasks && value == "deleted" {
				value = model.TicketStatusArchived
			}
			filter.Statuses = append(filter.Statuses, value)
		}
	}
	if tasks {
		pending := q.Get("pending")
		if pending != "" && pending != "true" && pending != "false" {
			kferr.Write(w, kferr.InvalidParameter("Invalid pending"))
			return
		}
		if pending == "true" {
			httpx.SetPagination(w, r, a.d.BaseURL, page, 0)
			httpx.WriteJSON(w, http.StatusOK, []any{})
			return
		}
	} else {
		if values, e := httpx.QueryArray(r, "subStatusIds", true); e != nil {
			kferr.Write(w, e)
			return
		} else {
			filter.SubStatusIDs = values
		}
		filter.AssigneeUserID = q.Get("assigneeUserId")
		if values, e := httpx.QueryArray(r, "assigneeStatus", true); e != nil {
			kferr.Write(w, e)
			return
		} else {
			for _, value := range values {
				if value != "all" && value != "current" && value != "approved" {
					kferr.Write(w, kferr.InvalidParameter("Invalid assigneeStatus"))
					return
				}
				filter.AssigneeStatuses = append(filter.AssigneeStatuses, value)
			}
		}
		if (filter.AssigneeUserID == "") != (len(filter.AssigneeStatuses) == 0) {
			kferr.Write(w, kferr.InvalidParameter("assigneeUserId and assigneeStatus must be specified together"))
			return
		}
		if filter.AssigneeUserID != "" && !store.IsUUID(filter.AssigneeUserID) {
			kferr.Write(w, kferr.InvalidParameter("Invalid assigneeUserId"))
			return
		}
	}
	filter.WorkflowID = q.Get("workflowId")
	filter.AuthorID = q.Get("authorId")
	filter.TicketNumber = q.Get("ticketNumber")
	filter.AuthorTeamFullName, err = httpx.QueryString(r, "authorTeamFullName", 300)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	filter.StepTitle, err = httpx.QueryString(r, "stepTitle", 300)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	datePairs := []struct {
		start, end string
		a, b       **time.Time
	}{
		{"createdAtStart", "createdAtEnd", &filter.CreatedAtStart, &filter.CreatedAtEnd}, {"updatedAtStart", "updatedAtEnd", &filter.UpdatedAtStart, &filter.UpdatedAtEnd}, {"openedAtStart", "openedAtEnd", &filter.OpenedAtStart, &filter.OpenedAtEnd}, {"completedAtStart", "completedAtEnd", &filter.CompletedAtStart, &filter.CompletedAtEnd}, {"archivedAtStart", "archivedAtEnd", &filter.ArchivedAtStart, &filter.ArchivedAtEnd},
	}
	if tasks {
		datePairs = append(datePairs, struct {
			start, end string
			a, b       **time.Time
		}{"assignedToMeAtStart", "assignedToMeAtEnd", &filter.AssignedToMeAtStart, &filter.AssignedToMeAtEnd})
	}
	for _, pair := range datePairs {
		*pair.a, err = httpx.QueryTime(r, pair.start)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		*pair.b, err = httpx.QueryTime(r, pair.end)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		if *pair.a != nil && *pair.b != nil && (*pair.a).After(**pair.b) {
			kferr.Write(w, kferr.InvalidParameter(pair.start+" must not be after "+pair.end))
			return
		}
	}
	sortBy := q.Get("sortBy")
	if sortBy != "" {
		parts := strings.Split(sortBy, "-")
		filter.SortBy = parts[0]
		if len(parts) > 2 || (len(parts) == 2 && parts[1] != "asc" && parts[1] != "desc") {
			kferr.Write(w, kferr.InvalidParameter("Invalid sortBy"))
			return
		}
		if len(parts) == 2 {
			filter.Desc = parts[1] == "desc"
		}
		allowed := []string{"createdAt", "updatedAt"}
		if tasks {
			allowed = append(allowed, "openedAt", "assignedToMeAt")
		}
		ok := false
		for _, v := range allowed {
			if filter.SortBy == v {
				ok = true
			}
		}
		if !ok {
			kferr.Write(w, kferr.InvalidParameter("Invalid sortBy"))
			return
		}
	}
	if tasks {
		filter.TaskUserIDs = []string{p.User.ID}
		filter.TaskProxyUserID = p.User.ID
		filter.TaskProxyDate = kftime.FormatDate(a.d.Clock.Now())
	}
	if !tasks && !p.Has(model.PermissionTicketRead, model.PermissionTicketWrite) {
		candidates, _, e := store.TicketQueryList(r.Context(), a.d.DB.Q(), filter, 0, -1)
		if e != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		visible := make([]string, 0, len(candidates))
		actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
		for _, id := range candidates {
			canViewErr := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, id)
			if canViewErr == nil {
				visible = append(visible, id)
				continue
			}
			apiErr, ok := kferr.As(canViewErr)
			if !ok || apiErr.Code != "invisible_ticket" {
				kferr.Write(w, kferr.Internal())
				return
			}
		}
		filter.VisibleTicketIDs = visible
	}
	ids, total, err := store.TicketQueryList(r.Context(), a.d.DB.Q(), filter, page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, err := present.TicketsWithStep(r.Context(), a.d.DB.Q(), ids)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) getTicket(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	id := r.PathValue("ticketId")
	if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}, id); err != nil {
		kferr.Write(w, err)
		return
	}
	out, err := present.TicketDetailFor(r.Context(), a.d.DB.Q(), id, pf16Caller(p.User.ID, p.Permissions))
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) createTicket(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	var input engine.CreateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	var out present.TicketDetailJSON
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		id, events, e := a.d.Engine.Create(r.Context(), tx, actor, input)
		if e != nil {
			return e
		}
		if e = store.InsertAuditLog(r.Context(), tx, pf22Audit(p.User.ID, "ticket.create", id, r)); e != nil {
			return e
		}
		if e = a.d.Events.Enqueue(r.Context(), tx, events); e != nil {
			return e
		}
		out, e = present.TicketDetailFor(r.Context(), tx, id, pf16Caller(p.User.ID, p.Permissions))
		return e
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) updateTicket(w http.ResponseWriter, r *http.Request) {
	var input engine.UpdateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		kferr.Write(w, err)
		return
	}
	a.pf22Update(w, r, "ticket.update", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.Update(ctx, tx, actor, id, input)
	})
}
func (a *API) updateTicketForm(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil || (r.ContentLength == 0 && len(r.TransferEncoding) == 0) {
		kferr.Write(w, kferr.InvalidParameter("form patch must not be empty"))
		return
	}
	var input engine.FormPatch
	if err := httpx.DecodeJSON(r, &input); err != nil {
		kferr.Write(w, err)
		return
	}
	if len(input.Inputs) == 0 && len(input.SlipItems) == 0 {
		kferr.Write(w, kferr.InvalidParameter("form patch must not be empty"))
		return
	}
	a.pf22Update(w, r, "ticket.updateForm", func(ctx context.Context, tx *store.Tx, actor engine.Actor, id string) ([]model.WebhookEvent, error) {
		return a.d.Engine.UpdateForm(ctx, tx, actor, id, input)
	})
}
func (a *API) pf22Update(w http.ResponseWriter, r *http.Request, action string, mutate func(context.Context, *store.Tx, engine.Actor, string) ([]model.WebhookEvent, error)) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	id := r.PathValue("ticketId")
	actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
	var out present.TicketDetailJSON
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		events, e := mutate(r.Context(), tx, actor, id)
		if e != nil {
			return e
		}
		if e = store.InsertAuditLog(r.Context(), tx, pf22Audit(p.User.ID, action, id, r)); e != nil {
			return e
		}
		if e = a.d.Events.Enqueue(r.Context(), tx, events); e != nil {
			return e
		}
		out, e = present.TicketDetailFor(r.Context(), tx, id, pf16Caller(p.User.ID, p.Permissions))
		return e
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Events.Notify()
	httpx.WriteJSON(w, http.StatusOK, out)
}

func pf22Audit(userID, action, ticketID string, r *http.Request) model.AuditLog {
	return model.AuditLog{ID: store.NewID(), UserID: &userID, Action: action, TicketID: &ticketID, CreatedAt: time.Now().UTC(), RemoteIP: pf22RemoteIP(r)}
}
func pf22RemoteIP(r *http.Request) *string {
	host := r.RemoteAddr
	if host == "" {
		return nil
	}
	return &host
}
