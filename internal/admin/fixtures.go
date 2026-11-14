package admin

import (
	"encoding/json"
	"net/http"

	"punchflow/internal/fixtures"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

func (a *API) exportFixtures(w http.ResponseWriter, r *http.Request) {
	if _, ok := pf24Principal(w, r, model.PermissionTenant); !ok {
		return
	}
	files, err := fixtures.Export(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, err)
		return
	}
	out := make(map[string]json.RawMessage, len(files))
	for name, content := range files {
		var raw json.RawMessage
		if err := json.Unmarshal(content, &raw); err != nil {
			kferr.Write(w, err)
			return
		}
		out[name] = raw
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) resetFixtures(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionTenant)
	if !ok {
		return
	}
	if !a.d.EnableReset {
		kferr.Write(w, kferr.EndpointNotFound())
		return
	}
	summary, err := fixtures.Reset(r.Context(), a.d.DB, a.d.FixturesFS, a.d.FilesDir, a.d.Clock)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if a.d.Faults != nil {
		a.d.Faults.Clear()
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"counts": summary.Counts, "message": "Fixture reset completed; use a fixture-seeded token for subsequent requests", "actorUserId": p.User.ID})
}
