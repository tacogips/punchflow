// Package admin defines the punchflow-only development administration API.
package admin

import (
	"io/fs"
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/store"
	"punchflow/internal/webhook"
)

// Deps contains the services used by admin handlers.
type Deps struct {
	DB          *store.DB
	Auth        *auth.Authenticator
	Webhooks    *webhook.Service
	Faults      *httpx.FaultSet
	Clock       kftime.Clock
	FixturesFS  fs.FS
	FilesDir    string
	EnableReset bool
	BaseURL     string
}

// API registers the developer administration routes.
type API struct{ d Deps }

// New constructs the admin API.
func New(d Deps) *API { return &API{d: d} }

// Operation is one row in the stable admin route contract.
type Operation struct {
	Name, Method, Pattern string
	Handle                func(a *API, w http.ResponseWriter, r *http.Request)
}

func (a *API) unavailable(w http.ResponseWriter, _ *http.Request) {
	kferr.Write(w, kferr.Internal())
}
