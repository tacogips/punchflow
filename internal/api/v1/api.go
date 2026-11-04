package v1

import (
	"context"
	"net/http"
	"punchflow/internal/engine"
	"punchflow/internal/files"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type EventSink interface {
	Enqueue(context.Context, store.Querier, []model.WebhookEvent) error
	Notify()
}
type Deps struct {
	DB      *store.DB
	Engine  *engine.Engine
	Events  EventSink
	Files   *files.Store
	Clock   kftime.Clock
	BaseURL string
}
type API struct{ d Deps }

func New(d Deps) *API { return &API{d: d} }

type Operation struct {
	OperationID string
	Method      string
	Pattern     string
	Handle      func(*API, http.ResponseWriter, *http.Request)
}
