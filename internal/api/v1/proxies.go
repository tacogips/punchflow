package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf19ProxyBody struct {
	UserID      string   `json:"userId"`
	ProxyUserID string   `json:"proxyUserId"`
	StartsOn    *string  `json:"startsOn"`
	EndsOn      *string  `json:"endsOn"`
	WorkflowIDs []string `json:"workflowIds"`
}

// pf19ProxyPatch is the PATCH body; startsOn and endsOn keep absent, null and
// concrete values apart so null can clear a bound.
type pf19ProxyPatch struct {
	UserID      string           `json:"userId"`
	ProxyUserID string           `json:"proxyUserId"`
	StartsOn    nullable[string] `json:"startsOn"`
	EndsOn      nullable[string] `json:"endsOn"`
	WorkflowIDs []string         `json:"workflowIds"`
}

func (a *API) listProxyApplicants(w http.ResponseWriter, r *http.Request) {
	a.pf19ListProxies(w, r, true)
}
func (a *API) listProxyApprovers(w http.ResponseWriter, r *http.Request) {
	a.pf19ListProxies(w, r, false)
}
func (a *API) pf19ListProxies(w http.ResponseWriter, r *http.Request, applicant bool) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	var values []any
	var total int
	if applicant {
		xs, n, e := store.ProxyListApplicants(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage)
		if e != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		total = n
		values = make([]any, 0, len(xs))
		for _, x := range xs {
			v, e := present.ProxyApplicant(r.Context(), a.d.DB.Q(), x.ID)
			if e != nil {
				kferr.Write(w, kferr.Internal())
				return
			}
			values = append(values, v)
		}
	} else {
		xs, n, e := store.ProxyListApprovers(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage)
		if e != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		total = n
		values = make([]any, 0, len(xs))
		for _, x := range xs {
			v, e := present.ProxyApprover(r.Context(), a.d.DB.Q(), x.ID)
			if e != nil {
				kferr.Write(w, kferr.Internal())
				return
			}
			values = append(values, v)
		}
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, values)
	_ = p
}
func (a *API) createProxyApplicant(w http.ResponseWriter, r *http.Request) {
	a.pf19CreateProxy(w, r, true)
}
func (a *API) createProxyApprover(w http.ResponseWriter, r *http.Request) {
	a.pf19CreateProxy(w, r, false)
}
func (a *API) pf19CreateProxy(w http.ResponseWriter, r *http.Request, applicant bool) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	var err error
	var b pf19ProxyBody
	if err = httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	if b.UserID == "" || b.ProxyUserID == "" {
		kferr.Write(w, kferr.InvalidParameter("userId and proxyUserId are required"))
		return
	}
	if !store.IsUUID(b.UserID) || !store.IsUUID(b.ProxyUserID) || !pf19ProxyWorkflowIDsAreUUIDs(b.WorkflowIDs) {
		kferr.Write(w, kferr.InvalidParameter("userId, proxyUserId, and workflowIds must contain UUIDs"))
		return
	}
	resolvedUser, resolveErr := store.ResolveID(r.Context(), a.d.DB.Q(), "users", b.UserID)
	if resolveErr != nil {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"userId": {"must reference an existing user"}}))
		return
	}
	if _, err = a.pf19ProxyPermission(r, resolvedUser, ""); err != nil {
		kferr.Write(w, err)
		return
	}
	var resultID string
	now := time.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		v, e := a.pf19NormalizeProxy(r, tx, b, p.User.ID, applicant, "")
		if e != nil {
			return e
		}
		if applicant {
			v.ID = store.NewID()
			v.CreatedAt = now
			v.UpdatedAt = now
			e = store.ProxyInsertApplicant(r.Context(), tx, v)
			resultID = v.ID
		} else {
			x := model.ProxyApprover{ID: store.NewID(), UserID: v.UserID, ProxyUserID: v.ProxyUserID, StartsOn: v.StartsOn, EndsOn: v.EndsOn, WorkflowIDs: v.WorkflowIDs, CreatedAt: now, UpdatedAt: now}
			e = store.ProxyInsertApprover(r.Context(), tx, x)
			resultID = x.ID
			if e == nil {
				e = a.pf19ShareProxyTickets(r.Context(), tx, x.UserID, x.WorkflowIDs, now)
			}
		}
		if e != nil {
			return a.pf19ProxyWriteError(e)
		}
		return a.pf19ProxyAudit(r, tx, p.User.ID, applicant, "create", resultID)
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	var out any
	if applicant {
		out, err = present.ProxyApplicant(r.Context(), a.d.DB.Q(), resultID)
	} else {
		out, err = present.ProxyApprover(r.Context(), a.d.DB.Q(), resultID)
	}
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) updateProxyApplicant(w http.ResponseWriter, r *http.Request) {
	a.pf19UpdateProxy(w, r, true)
}
func (a *API) updateProxyApprover(w http.ResponseWriter, r *http.Request) {
	a.pf19UpdateProxy(w, r, false)
}
func (a *API) pf19UpdateProxy(w http.ResponseWriter, r *http.Request, applicant bool) {
	id := r.PathValue("proxyApplicantId")
	table := "proxy_approvers"
	if applicant {
		table = "proxy_applicants"
		id = r.PathValue("proxyApplicantId")
	}
	if id == "" {
		id = r.PathValue("proxyApproverId")
	}
	var currentUser string
	if applicant {
		x, e := store.GetProxyApplicant(r.Context(), a.d.DB.Q(), id)
		if e != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		currentUser = x.UserID
	} else {
		x, e := store.GetProxyApprover(r.Context(), a.d.DB.Q(), id)
		if e != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		currentUser = x.UserID
	}
	p, err := a.pf19ProxyPermission(r, currentUser, id)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	var b pf19ProxyPatch
	if err = httpx.DecodeJSON(r, &b); err != nil {
		kferr.Write(w, err)
		return
	}
	if (b.UserID != "" && !store.IsUUID(b.UserID)) || (b.ProxyUserID != "" && !store.IsUUID(b.ProxyUserID)) || (b.WorkflowIDs != nil && !pf19ProxyWorkflowIDsAreUUIDs(b.WorkflowIDs)) {
		kferr.Write(w, kferr.InvalidParameter("userId, proxyUserId, and workflowIds must contain UUIDs"))
		return
	}
	now := time.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if applicant {
			cur, e := store.GetProxyApplicant(r.Context(), tx, id)
			if e != nil {
				return kferr.ResourceNotFound()
			}
			if b.UserID != "" {
				cur.UserID = b.UserID
			}
			if b.ProxyUserID != "" {
				cur.ProxyUserID = b.ProxyUserID
			}
			b.StartsOn.applyTo(&cur.StartsOn)
			b.EndsOn.applyTo(&cur.EndsOn)
			if b.WorkflowIDs != nil {
				cur.WorkflowIDs = b.WorkflowIDs
			}
			if !p.Has(model.PermissionUser) && cur.UserID != p.User.ID {
				return kferr.MissingPermission("Missing permission")
			}
			norm, e := a.pf19NormalizeProxy(r, tx, pf19ProxyBody{cur.UserID, cur.ProxyUserID, cur.StartsOn, cur.EndsOn, cur.WorkflowIDs}, p.User.ID, true, id)
			if e != nil {
				return e
			}
			norm.ID = id
			norm.CreatedAt = cur.CreatedAt
			norm.UpdatedAt = now
			if e = store.ProxyUpdateApplicant(r.Context(), tx, id, norm); e != nil {
				return a.pf19ProxyWriteError(e)
			}
		} else {
			cur, e := store.GetProxyApprover(r.Context(), tx, id)
			if e != nil {
				return kferr.ResourceNotFound()
			}
			if b.UserID != "" {
				cur.UserID = b.UserID
			}
			if b.ProxyUserID != "" {
				cur.ProxyUserID = b.ProxyUserID
			}
			b.StartsOn.applyTo(&cur.StartsOn)
			b.EndsOn.applyTo(&cur.EndsOn)
			if b.WorkflowIDs != nil {
				cur.WorkflowIDs = b.WorkflowIDs
			}
			if !p.Has(model.PermissionUser) && cur.UserID != p.User.ID {
				return kferr.MissingPermission("Missing permission")
			}
			app := model.ProxyApplicant{UserID: cur.UserID, ProxyUserID: cur.ProxyUserID, StartsOn: cur.StartsOn, EndsOn: cur.EndsOn, WorkflowIDs: cur.WorkflowIDs}
			norm, e := a.pf19NormalizeProxy(r, tx, pf19ProxyBody{app.UserID, app.ProxyUserID, app.StartsOn, app.EndsOn, app.WorkflowIDs}, p.User.ID, false, id)
			if e != nil {
				return e
			}
			x := model.ProxyApprover{ID: id, UserID: norm.UserID, ProxyUserID: norm.ProxyUserID, StartsOn: norm.StartsOn, EndsOn: norm.EndsOn, WorkflowIDs: norm.WorkflowIDs, CreatedAt: cur.CreatedAt, UpdatedAt: now}
			if e = store.ProxyUpdateApprover(r.Context(), tx, id, x); e != nil {
				return a.pf19ProxyWriteError(e)
			}
			if e = a.pf19ShareProxyTickets(r.Context(), tx, x.UserID, x.WorkflowIDs, now); e != nil {
				return e
			}
		}
		return a.pf19ProxyAudit(r, tx, p.User.ID, applicant, "update", id)
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	var out any
	if applicant {
		out, err = present.ProxyApplicant(r.Context(), a.d.DB.Q(), id)
	} else {
		out, err = present.ProxyApprover(r.Context(), a.d.DB.Q(), id)
	}
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	_ = table
}

// pf19ShareProxyTickets persists the share and watch side effects for a newly
// effective proxy-approver setting. Existing rows are never removed when a
// setting later expires or is edited away.
func (a *API) pf19ShareProxyTickets(ctx context.Context, tx *store.Tx, principalID string, workflows []string, now time.Time) error {
	tickets, err := store.EngProcessingTicketIDsForAssignee(ctx, tx, principalID)
	if err != nil {
		return err
	}
	for _, ticketID := range tickets {
		ticket, err := store.EngLoadTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if len(workflows) > 0 {
			allowed := false
			for _, workflowID := range workflows {
				if workflowID == ticket.WorkflowID {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		if err = engine.SyncProxySharingForTicket(ctx, tx, ticketID, now); err != nil {
			return err
		}
	}
	return nil
}
func (a *API) deleteProxyApplicant(w http.ResponseWriter, r *http.Request) {
	a.pf19DeleteProxy(w, r, true)
}
func (a *API) deleteProxyApprover(w http.ResponseWriter, r *http.Request) {
	a.pf19DeleteProxy(w, r, false)
}
func (a *API) pf19DeleteProxy(w http.ResponseWriter, r *http.Request, applicant bool) {
	id := r.PathValue("proxyApplicantId")
	user := ""
	if applicant {
		x, e := store.GetProxyApplicant(r.Context(), a.d.DB.Q(), id)
		if e != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		user = x.UserID
	} else {
		id = r.PathValue("proxyApproverId")
		x, e := store.GetProxyApprover(r.Context(), a.d.DB.Q(), id)
		if e != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		user = x.UserID
	}
	p, e := a.pf19ProxyPermission(r, user, id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var er error
		if applicant {
			er = store.ProxyDeleteApplicant(r.Context(), tx, id)
		} else {
			er = store.ProxyDeleteApprover(r.Context(), tx, id)
		}
		if er != nil {
			return kferr.ResourceNotFound()
		}
		return a.pf19ProxyAudit(r, tx, p.User.ID, applicant, "delete", id)
	})
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) pf19ProxyPermission(r *http.Request, userID, proxyID string) (*auth.Principal, error) {
	if err := auth.Require(r); err != nil {
		return nil, err
	}
	p, _ := auth.FromContext(r.Context())
	if p.Has(model.PermissionUser) {
		return p, nil
	}
	if userID == "" {
		return nil, kferr.MissingPermission("Missing permission")
	}
	var enabled int
	err := a.d.DB.Q().QueryRowContext(r.Context(), "SELECT self_service_proxy FROM tenant LIMIT 1").Scan(&enabled)
	if err != nil || enabled == 0 || userID != p.User.ID {
		return nil, kferr.MissingPermission("Missing permission")
	}
	if proxyID != "" {
		var target string
		err = a.d.DB.Q().QueryRowContext(r.Context(), "SELECT user_id FROM proxy_applicants WHERE id=? UNION ALL SELECT user_id FROM proxy_approvers WHERE id=? LIMIT 1", proxyID, proxyID).Scan(&target)
		if err == nil && target != p.User.ID {
			return nil, kferr.MissingPermission("Missing permission")
		}
	}
	return p, nil
}
func (a *API) pf19NormalizeProxy(r *http.Request, q store.Querier, b pf19ProxyBody, actor string, applicant bool, excludeID string) (model.ProxyApplicant, error) {
	uid, e := store.ResolveID(r.Context(), q, "users", b.UserID)
	if e != nil {
		return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"userId": {"must reference an existing user"}})
	}
	pid, e := store.ResolveID(r.Context(), q, "users", b.ProxyUserID)
	if e != nil {
		return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"proxyUserId": {"must reference an existing user"}})
	}
	if uid == pid {
		return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"proxyUserId": {"must differ from userId"}})
	}
	for _, id := range []string{uid, pid} {
		var status string
		if e = q.QueryRowContext(r.Context(), "SELECT status FROM users WHERE id=?", id).Scan(&status); e != nil || status != model.UserStatusActive {
			return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"proxyUserId": {"must reference an active user"}})
		}
	}
	if b.StartsOn != nil {
		if _, e = time.Parse("2006-01-02", *b.StartsOn); e != nil {
			return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"startsOn": {"must be a date"}})
		}
	}
	if b.EndsOn != nil {
		if _, e = time.Parse("2006-01-02", *b.EndsOn); e != nil {
			return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"endsOn": {"must be a date"}})
		}
	}
	if b.StartsOn != nil && b.EndsOn != nil && *b.StartsOn > *b.EndsOn {
		return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"startsOn": {"must not be after endsOn"}})
	}
	workflows := []string{}
	seen := map[string]bool{}
	for _, raw := range b.WorkflowIDs {
		var wid string
		e := q.QueryRowContext(r.Context(), "SELECT id FROM workflows WHERE lower(id)=lower(?) OR lower(code)=lower(?) LIMIT 1", raw, raw).Scan(&wid)
		if e != nil {
			return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"workflowIds": {"must reference existing workflows"}})
		}
		key := strings.ToLower(wid)
		if !seen[key] {
			workflows = append(workflows, wid)
			seen[key] = true
		}
	}
	table := "proxy_approvers"
	if applicant {
		table = "proxy_applicants"
	}
	var count int
	if e = q.QueryRowContext(r.Context(), "SELECT count(*) FROM "+table+" WHERE user_id=? AND proxy_user_id=? AND id<>?", uid, pid, excludeID).Scan(&count); e != nil {
		return model.ProxyApplicant{}, kferr.Internal()
	}
	if count > 0 {
		return model.ProxyApplicant{}, kferr.ValidationFailed(map[string][]string{"proxyUserId": {"pair already exists"}})
	}
	return model.ProxyApplicant{UserID: uid, ProxyUserID: pid, StartsOn: b.StartsOn, EndsOn: b.EndsOn, WorkflowIDs: workflows}, nil
}

func pf19ProxyWorkflowIDsAreUUIDs(ids []string) bool {
	for _, id := range ids {
		if !store.IsUUID(id) {
			return false
		}
	}
	return true
}
func (a *API) pf19ProxyAudit(r *http.Request, q store.Querier, user string, applicant bool, verb, id string) error {
	kind := "proxyApprover"
	if applicant {
		kind = "proxyApplicant"
	}
	data, _ := json.Marshal(map[string]string{"id": id})
	u := user
	remote := httpx.ClientIP(r)
	rid := httpx.RequestIDFrom(r.Context())
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &u, Action: kind + "." + verb, Data: data, RemoteIP: &remote, RequestID: &rid, CreatedAt: time.Now()})
}
func (a *API) pf19ProxyWriteError(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return kferr.ValidationFailed(map[string][]string{"proxyUserId": {"pair already exists"}})
	}
	return kferr.Internal()
}
