package admin

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"

	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) getTenant(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionTenant); !ok {
		return
	}
	t, err := store.Tenant(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24TenantJSON(t))
}
func (a *API) updateTenant(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionTenant)
	if !ok {
		return
	}
	var patch map[string]json.RawMessage
	if err := httpx.DecodeJSON(r, &patch); err != nil {
		kferr.Write(w, err)
		return
	}
	t, err := store.Tenant(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, err)
		return
	}
	decode := func(key string, dst any) error {
		raw, exists := patch[key]
		if !exists {
			return nil
		}
		return json.Unmarshal(raw, dst)
	}
	if err = decode("name", &t.Name); err == nil {
		err = decode("publicId", &t.PublicID)
	}
	if err == nil {
		err = decode("plan", &t.Plan)
	}
	if err == nil {
		err = decode("mode", &t.Mode)
	}
	if err == nil {
		err = decode("ipAllowlist", &t.IPAllowlist)
	}
	if err == nil {
		err = decode("allowedEmailDomains", &t.AllowedEmailDomains)
	}
	if err == nil {
		err = decode("selfServiceProxy", &t.SelfServiceProxy)
	}
	if err != nil {
		kferr.Write(w, kferr.InvalidParameter("Invalid tenant setting"))
		return
	}
	if t.Name == "" || t.PublicID == "" || !pf24OneOf(t.Plan, model.TenantPlanStandard, model.TenantPlanEnterprise) || !pf24OneOf(t.Mode, model.TenantModeNormal, model.TenantModeAdminWorking, model.TenantModeMaintenance, model.TenantModeSuspended) {
		kferr.Write(w, kferr.InvalidParameter("Invalid tenant setting"))
		return
	}
	for _, v := range t.IPAllowlist {
		if ip, err := netip.ParseAddr(v); err != nil {
			if _, e := netip.ParsePrefix(v); e != nil {
				kferr.Write(w, kferr.ValidationFailed(map[string][]string{"ipAllowlist": {"must contain IP addresses or CIDRs"}}))
				return
			}
		} else if ip.Zone() != "" {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"ipAllowlist": {"must contain unzoned IP addresses"}}))
			return
		}
	}
	for _, domain := range t.AllowedEmailDomains {
		d := strings.TrimSpace(domain)
		if d == "" || strings.Contains(d, "@") || strings.ContainsAny(d, " /\\") {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"allowedEmailDomains": {"must contain domain names without @"}}))
			return
		}
	}
	t.UpdatedAt = a.d.Clock.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if e := store.AdminUpdateTenant(r.Context(), tx, t); e != nil {
			return e
		}
		return pf24Audit(r.Context(), tx, p, "tenant.update", patch)
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pf24TenantJSON(t))
}
func pf24OneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

func pf24TenantJSON(t model.Tenant) map[string]any {
	return map[string]any{"id": t.ID, "publicId": t.PublicID, "name": t.Name, "plan": t.Plan, "mode": t.Mode, "ipAllowlist": t.IPAllowlist, "allowedEmailDomains": t.AllowedEmailDomains, "selfServiceProxy": t.SelfServiceProxy, "createdAt": kftime.Format(t.CreatedAt), "updatedAt": kftime.Format(t.UpdatedAt)}
}
