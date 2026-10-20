package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionIntegration); !ok {
		return
	}
	page, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	rows, total, e := store.ListWebhooks(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, pf24WebhookJSON(v, false))
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	var in struct {
		Name        string   `json:"name"`
		URL         string   `json:"url"`
		Secret      *string  `json:"secret"`
		Enabled     bool     `json:"enabled"`
		WorkflowIDs []string `json:"workflowIds"`
		EventTypes  []string `json:"eventTypes"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		kferr.Write(w, err)
		return
	}
	if in.Name == "" || !pf24ValidWebhookURL(in.URL) {
		kferr.Write(w, kferr.InvalidParameter("name and an absolute http(s) url are required"))
		return
	}
	if in.WorkflowIDs == nil {
		in.WorkflowIDs = []string{}
	}
	if in.EventTypes == nil {
		in.EventTypes = []string{}
	}
	if err := pf24ValidateHookFilters(r.Context(), a.d.DB.Q(), in.WorkflowIDs, in.EventTypes); err != nil {
		kferr.Write(w, err)
		return
	}
	now := a.d.Clock.Now()
	v := model.Webhook{ID: store.NewID(), Name: in.Name, URL: in.URL, Secret: in.Secret, Enabled: in.Enabled, WorkflowIDs: in.WorkflowIDs, EventTypes: in.EventTypes, CreatedAt: now, UpdatedAt: now}
	var eventID string
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.InsertWebhook(r.Context(), tx, v); e != nil {
			return e
		}
		ids, e := a.d.Webhooks.EnqueueTo(r.Context(), tx, model.WebhookEvent{Type: model.WebhookEventPing, ActorUserID: p.User.ID}, []string{v.ID})
		if e != nil {
			return e
		}
		eventID = ids[0]
		return pf24Audit(r.Context(), tx, p, "webhook.create", map[string]string{"webhookId": v.ID})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	a.d.Webhooks.Notify()
	out := pf24WebhookJSON(v, true)
	out["secret"] = in.Secret
	out["pingEventId"] = eventID
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getWebhook(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionIntegration); !ok {
		return
	}
	v, e := store.GetWebhook(r.Context(), a.d.DB.Q(), r.PathValue("id"))
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24WebhookJSON(v, false))
}
func (a *API) updateWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	id := r.PathValue("id")
	v, e := store.GetWebhook(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	var patch map[string]json.RawMessage
	if e = httpx.DecodeJSON(r, &patch); e != nil {
		kferr.Write(w, e)
		return
	}
	beforeURL := v.URL
	beforeSecret := ""
	if v.Secret != nil {
		beforeSecret = *v.Secret
	}
	decode := func(k string, dst any) error {
		raw, exists := patch[k]
		if !exists {
			return nil
		}
		return json.Unmarshal(raw, dst)
	}
	if e = decode("name", &v.Name); e == nil {
		e = decode("url", &v.URL)
	}
	if e == nil {
		e = decode("secret", &v.Secret)
	}
	if e == nil {
		e = decode("enabled", &v.Enabled)
	}
	if e == nil {
		e = decode("workflowIds", &v.WorkflowIDs)
	}
	if e == nil {
		e = decode("eventTypes", &v.EventTypes)
	}
	if e != nil || v.Name == "" || !pf24ValidWebhookURL(v.URL) {
		kferr.Write(w, kferr.InvalidParameter("Invalid webhook fields"))
		return
	}
	if v.WorkflowIDs == nil {
		v.WorkflowIDs = []string{}
	}
	if v.EventTypes == nil {
		v.EventTypes = []string{}
	}
	if e = pf24ValidateHookFilters(r.Context(), a.d.DB.Q(), v.WorkflowIDs, v.EventTypes); e != nil {
		kferr.Write(w, e)
		return
	}
	now := a.d.Clock.Now()
	v.UpdatedAt = now
	if v.Enabled {
		v.DisabledAt = nil
		v.FirstFailureAt = nil
	}
	changed := beforeURL != v.URL || beforeSecret != pf24Secret(v.Secret)
	var eventID string
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if x := store.UpdateWebhook(r.Context(), tx, v); x != nil {
			return x
		}
		if changed {
			ids, x := a.d.Webhooks.EnqueueTo(r.Context(), tx, model.WebhookEvent{Type: model.WebhookEventPing, ActorUserID: p.User.ID}, []string{id})
			if x != nil {
				return x
			}
			eventID = ids[0]
		}
		return pf24Audit(r.Context(), tx, p, "webhook.update", map[string]string{"webhookId": id})
	})
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	if changed {
		a.d.Webhooks.Notify()
	}
	out := pf24WebhookJSON(v, true)
	out["secret"] = v.Secret
	if eventID != "" {
		out["pingEventId"] = eventID
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	id := r.PathValue("id")
	e := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if x := store.DeleteWebhook(r.Context(), tx, id); x != nil {
			return x
		}
		return pf24Audit(r.Context(), tx, p, "webhook.delete", map[string]string{"webhookId": id})
	})
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) pingWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var ids []string
	e := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var x error
		ids, x = a.d.Webhooks.EnqueueTo(r.Context(), tx, model.WebhookEvent{Type: model.WebhookEventPing, ActorUserID: p.User.ID}, []string{id})
		if x != nil {
			return x
		}
		return pf24Audit(r.Context(), tx, p, "webhook.ping", map[string]string{"webhookId": id})
	})
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	a.d.Webhooks.Notify()
	v, e := store.GetWebhookEvent(r.Context(), a.d.DB.Q(), ids[0])
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24DeliveryJSON(v, false, nil))
}
func (a *API) listWebhookEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionIntegration); !ok {
		return
	}
	id := r.PathValue("id")
	if _, e := store.GetWebhook(r.Context(), a.d.DB.Q(), id); e != nil {
		pf24StoreError(w, e)
		return
	}
	page, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	rows, total, e := store.ListWebhookEvents(r.Context(), a.d.DB.Q(), id, page.Offset(), page.PerPage)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, pf24DeliveryJSON(v, false, nil))
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getWebhookEvent(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionIntegration); !ok {
		return
	}
	v, e := store.GetWebhookEvent(r.Context(), a.d.DB.Q(), r.PathValue("eventId"))
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	attempts, e := store.ListWebhookAttempts(r.Context(), a.d.DB.Q(), v.ID)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24DeliveryJSON(v, true, attempts))
}
func (a *API) resendWebhookEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	id := r.PathValue("eventId")
	e := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if x := a.d.Webhooks.Resend(r.Context(), tx, id); x != nil {
			return x
		}
		return pf24Audit(r.Context(), tx, p, "webhookEvent.resend", map[string]string{"eventId": id})
	})
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	v, e := store.GetWebhookEvent(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24DeliveryJSON(v, false, nil))
}
func (a *API) createWebhookEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionIntegration)
	if !ok {
		return
	}
	var in struct {
		EventType  string   `json:"eventType"`
		TicketID   string   `json:"ticketId"`
		CommentID  string   `json:"commentId"`
		WebhookIDs []string `json:"webhookIds"`
	}
	if e := httpx.DecodeJSON(r, &in); e != nil {
		kferr.Write(w, e)
		return
	}
	if !pf24Has(model.WebhookEventTypes, in.EventType) || in.EventType == model.WebhookEventPing && len(in.WebhookIDs) == 0 {
		kferr.Write(w, kferr.InvalidParameter("Invalid eventType"))
		return
	}
	if strings.HasPrefix(in.EventType, "ticket_") || strings.HasPrefix(in.EventType, "comment_") {
		if in.TicketID == "" {
			kferr.Write(w, kferr.InvalidParameter("ticketId is required"))
			return
		}
		var exists int
		if e := a.d.DB.Q().QueryRowContext(r.Context(), "SELECT count(*) FROM tickets WHERE id=?", in.TicketID).Scan(&exists); e != nil {
			kferr.Write(w, e)
			return
		}
		if exists == 0 {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
	}
	if strings.HasPrefix(in.EventType, "comment_") {
		if in.CommentID == "" {
			kferr.Write(w, kferr.InvalidParameter("commentId is required"))
			return
		}
		var belongs int
		if e := a.d.DB.Q().QueryRowContext(r.Context(), "SELECT count(*) FROM comments WHERE id=? AND ticket_id=? AND deleted_at IS NULL", in.CommentID, in.TicketID).Scan(&belongs); e != nil {
			kferr.Write(w, e)
			return
		}
		if belongs == 0 {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
	} else if in.CommentID != "" {
		kferr.Write(w, kferr.InvalidParameter("commentId is only valid for comment events"))
		return
	}
	ev := model.WebhookEvent{Type: in.EventType, TicketID: in.TicketID, CommentID: in.CommentID, ActorUserID: p.User.ID}
	var ids []string
	e := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var x error
		if in.WebhookIDs != nil {
			ids, x = a.d.Webhooks.EnqueueTo(r.Context(), tx, ev, in.WebhookIDs)
		} else {
			x = a.d.Webhooks.Enqueue(r.Context(), tx, []model.WebhookEvent{ev})
		}
		if x != nil {
			return x
		}
		return pf24Audit(r.Context(), tx, p, "webhookEvent.fire", map[string]string{"eventType": in.EventType, "ticketId": in.TicketID, "commentId": in.CommentID})
	})
	if e != nil {
		pf24StoreError(w, e)
		return
	}
	a.d.Webhooks.Notify()
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		v, x := store.GetWebhookEvent(r.Context(), a.d.DB.Q(), id)
		if x != nil {
			kferr.Write(w, x)
			return
		}
		out = append(out, pf24DeliveryJSON(v, false, nil))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func pf24WebhookJSON(v model.Webhook, includeSecret bool) map[string]any {
	m := map[string]any{"id": v.ID, "name": v.Name, "url": v.URL, "hasSecret": v.Secret != nil && *v.Secret != "", "enabled": v.Enabled, "workflowIds": v.WorkflowIDs, "eventTypes": v.EventTypes, "firstFailureAt": pf24TimePtr(v.FirstFailureAt), "disabledAt": pf24TimePtr(v.DisabledAt), "createdAt": kftime.Format(v.CreatedAt), "updatedAt": kftime.Format(v.UpdatedAt)}
	if includeSecret {
		m["secret"] = v.Secret
	}
	return m
}
func pf24DeliveryJSON(v model.WebhookDelivery, detail bool, attempts []model.WebhookAttempt) map[string]any {
	m := map[string]any{"id": v.ID, "webhookId": v.WebhookID, "eventType": v.EventType, "ticketId": v.TicketID, "commentId": v.CommentID, "status": v.Status, "attemptCount": v.AttemptCount, "nextAttemptAt": kftime.Format(v.NextAttemptAt), "lastStatusCode": v.LastStatusCode, "createdAt": kftime.Format(v.CreatedAt)}
	if detail {
		m["body"] = string(v.Body)
		list := make([]map[string]any, 0, len(attempts))
		for _, x := range attempts {
			list = append(list, map[string]any{"attemptNo": x.AttemptNo, "startedAt": kftime.Format(x.StartedAt), "durationMs": x.DurationMS, "requestHeaders": x.RequestHeaders, "statusCode": x.StatusCode, "responseBody": string(x.ResponseBody), "error": x.Error, "manual": x.Manual})
		}
		m["attempts"] = list
	}
	return m
}
func pf24TimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return kftime.Format(*t)
}
func pf24Secret(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func pf24ValidWebhookURL(raw string) bool {
	u, e := url.ParseRequestURI(raw)
	return e == nil && u.IsAbs() && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func pf24ValidateHookFilters(ctx context.Context, q store.Querier, workflows, events []string) error {
	seen := map[string]bool{}
	for _, ev := range events {
		if !pf24Has(model.WebhookEventTypes[1:], ev) || seen[ev] {
			return kferr.ValidationFailed(map[string][]string{"eventTypes": {"contains an invalid or duplicate event type"}})
		}
		seen[ev] = true
	}
	for _, id := range workflows {
		var n int
		if e := q.QueryRowContext(ctx, "SELECT count(*) FROM workflows WHERE id=?", id).Scan(&n); e != nil {
			return e
		}
		if n == 0 {
			return kferr.ValidationFailed(map[string][]string{"workflowIds": {"contains an unknown workflow"}})
		}
	}
	return nil
}
func pf24Has(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}
func pf24StoreError(w http.ResponseWriter, e error) {
	if errors.Is(e, store.ErrNotFound) || errors.Is(e, sql.ErrNoRows) {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	kferr.Write(w, e)
}
