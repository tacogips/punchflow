package admin

import (
	"net/http"
	"strings"

	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
)

type pf24FaultJSON struct {
	ID         string `json:"id"`
	Method     string `json:"method,omitempty"`
	PathPrefix string `json:"pathPrefix"`
	Status     int    `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message,omitempty"`
	Remaining  int    `json:"remaining"`
	CreatedAt  string `json:"createdAt"`
}

func (a *API) listFaults(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionTenant); !ok {
		return
	}
	rules := a.d.Faults.List()
	out := make([]pf24FaultJSON, 0, len(rules))
	for _, x := range rules {
		out = append(out, pf24FaultJSON{ID: x.ID, Method: x.Method, PathPrefix: x.PathPrefix, Status: x.Status, Code: x.Code, Message: x.Message, Remaining: x.Remaining, CreatedAt: kftime.Format(x.CreatedAt)})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createFault(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionTenant)
	if !ok {
		return
	}
	var in struct {
		Method     string `json:"method"`
		PathPrefix string `json:"pathPrefix"`
		Status     int    `json:"status"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		Remaining  int    `json:"remaining"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		kferr.Write(w, err)
		return
	}
	if in.Status < 400 || in.Status > 599 || in.Code == "" || !strings.HasPrefix(in.PathPrefix, "/v1/") || in.Remaining < 0 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"fault": {"status, code, pathPrefix and remaining are invalid"}}))
		return
	}
	allowed := map[string]int{"feature_disabled": 503, "gateway_timeout": 504, "email_not_verified": 401, "license_purchase_required": 403, "option_purchase_required": 403, "internal_server_error": 500}
	if status, exists := allowed[in.Code]; exists && status != in.Status {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"status": {"does not match the fault code"}}))
		return
	}
	rule := a.d.Faults.Add(httpx.FaultRule{Method: in.Method, PathPrefix: in.PathPrefix, Status: in.Status, Code: in.Code, Message: in.Message, Remaining: in.Remaining, CreatedAt: a.d.Clock.Now()})
	if err := pf24Audit(r.Context(), a.d.DB.Q(), p, "fault.create", map[string]string{"faultId": rule.ID}); err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24FaultJSON{ID: rule.ID, Method: rule.Method, PathPrefix: rule.PathPrefix, Status: rule.Status, Code: rule.Code, Message: rule.Message, Remaining: rule.Remaining, CreatedAt: kftime.Format(rule.CreatedAt)})
}
func (a *API) deleteFault(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionTenant)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !a.d.Faults.Delete(id) {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if err := pf24Audit(r.Context(), a.d.DB.Q(), p, "fault.delete", map[string]string{"faultId": id}); err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
