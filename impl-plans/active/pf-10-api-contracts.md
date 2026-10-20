# PF-10 API Contracts Implementation Plan

**Status**: In Progress
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-10
**Design Reference**: design-docs/specs/design-api-coverage.md#operation-matrix-96-operations-57-paths; design-docs/specs/design-approval-engine.md#engine-surface; design-docs/specs/architecture.md#files
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Pin, as compilable stubs, the contracts that wave 3/4 plans implement in parallel:
1. The `/v1` route table (all 96 operations), the `API` struct and its dependencies, and one stub handler per operation, spread over the 15 handler files named in the coverage matrix.
2. The engine surface (`internal/engine/engine.go`): types and method signatures with stub bodies.
3. The file store surface (`internal/files/files.go`).

Later plans replace the stub bodies without changing these signatures.

## Non-goals

- No real handler, engine or file logic. No middleware or server wiring (PF-16).
- Do not import `internal/webhook`, `internal/auth`, `internal/httpx` or `internal/present` here (same or later wave). Use the `EventSink` interface.

## Files

`internal/api/v1/api.go`, `routes.go`, `routes_test.go`, stub files `users.go`, `grades.go`, `proxies.go`, `orgcharts.go`, `teams.go`, `categories.go`, `folders.go`, `definitions.go`, `tickets.go`, `ticket_actions.go`, `ticket_viewers.go`, `files.go`, `general_masters.go`, `roles.go`, `comments.go`, `audit_logs.go`; `internal/engine/engine.go`; `internal/files/files.go`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-10-api-contracts.md`
- `internal/api/v1/api.go`
- `internal/api/v1/routes.go`
- `internal/api/v1/routes_test.go`
- `internal/api/v1/users.go`
- `internal/api/v1/grades.go`
- `internal/api/v1/proxies.go`
- `internal/api/v1/orgcharts.go`
- `internal/api/v1/teams.go`
- `internal/api/v1/categories.go`
- `internal/api/v1/folders.go`
- `internal/api/v1/definitions.go`
- `internal/api/v1/tickets.go`
- `internal/api/v1/ticket_actions.go`
- `internal/api/v1/ticket_viewers.go`
- `internal/api/v1/files.go`
- `internal/api/v1/general_masters.go`
- `internal/api/v1/roles.go`
- `internal/api/v1/comments.go`
- `internal/api/v1/audit_logs.go`
- `internal/engine/engine.go`
- `internal/files/files.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

```go
// internal/api/v1/api.go
type EventSink interface {
    Enqueue(ctx context.Context, q store.Querier, evs []model.WebhookEvent) error
    Notify()
}
type Deps struct {
    DB      *store.DB
    Engine  *engine.Engine
    Events  EventSink
    Files   *files.Store
    Clock   kftime.Clock
    BaseURL string // --public-base-url, no trailing slash
}
type API struct{ d Deps }
func New(d Deps) *API
type Operation struct {
    OperationID string
    Method      string // GET POST PATCH DELETE
    Pattern     string // e.g. "/v1/users/{userId}" (ServeMux wildcard syntax, names as in the spec)
    Handle      func(a *API, w http.ResponseWriter, r *http.Request)
}
func Operations() []Operation                       // exactly 96, in coverage-matrix order
func (a *API) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) // "METHOD PATTERN" per op + "/v1/" catch-all -> 404 endpoint_not_found
```

Handler methods are unexported and named exactly like the operationId: `func (a *API) getCurrentUser(w http.ResponseWriter, r *http.Request)`, `listUsers`, ..., `listAuditLogs`. Each stub writes `kferr.Write(w, kferr.New(500, "internal_server_error", "not implemented: <operationId>"))`. Methods live in the file given by the coverage matrix "Handler" column.

```go
// internal/engine/engine.go
type Actor struct{ UserID string; Permissions map[string]bool }
type Engine struct{ /* clock kftime.Clock */ }
func New(clock kftime.Clock) *Engine

type FieldInput struct {
    FormFieldID         string          `json:"formFieldId"`
    FormFieldCode       string          `json:"formFieldCode"`
    Value               json.RawMessage `json:"value"`
    GeneralMasterItemID json.RawMessage `json:"generalMasterItemId"`
    UserID              json.RawMessage `json:"userId"`
    TeamID              json.RawMessage `json:"teamId"`
    TicketID            json.RawMessage `json:"ticketId"`
    Files               []string        `json:"files"`
}
type SlipFieldInput struct { SlipFieldID string `json:"slipFieldId"`; SlipFieldCode string `json:"slipFieldCode"`; /* same value fields as FieldInput */ }
type SlipItemInput struct { SlipSectionID string `json:"slipSectionId"`; SlipSectionCode string `json:"slipSectionCode"`; Inputs []SlipFieldInput `json:"inputs"` }
type ApproverInput struct { RouteStepCode string `json:"routeStepCode"`; UserIDs []string `json:"userId"` }
type CreateInput struct {
    Status string `json:"status"`; WorkflowID string `json:"workflowId"`; AuthorTeamID string `json:"authorTeamId"`
    ProxyClientUserID *string `json:"proxyClientUserId"`; Title *string `json:"title"`
    Inputs *[]FieldInput `json:"inputs"`; SlipItems *[]SlipItemInput `json:"slipItems"`; Approvers []ApproverInput `json:"approvers"`
}
type UpdateInput struct {
    Status *string `json:"status"`; AuthorTeamID *string `json:"authorTeamId"`
    ProxyClientUserID json.RawMessage `json:"proxyClientUserId"` // absent vs null distinguishable
    Title *string `json:"title"`; Inputs *[]FieldInput `json:"inputs"`; SlipItems *[]SlipItemInput `json:"slipItems"`
    Approvers *[]ApproverInput `json:"approvers"`
}
type FormPatch struct { Inputs []FieldInput `json:"inputs"`; SlipItems []SlipItemInput `json:"slipItems"` }

func (e *Engine) Create(ctx context.Context, tx *store.Tx, a Actor, in CreateInput) (string, []model.WebhookEvent, error)
func (e *Engine) Update(ctx context.Context, tx *store.Tx, a Actor, ticketID string, in UpdateInput) ([]model.WebhookEvent, error)
func (e *Engine) UpdateForm(ctx context.Context, tx *store.Tx, a Actor, ticketID string, in FormPatch) ([]model.WebhookEvent, error)
func (e *Engine) Approve(ctx context.Context, tx *store.Tx, a Actor, ticketID string, comment *string) ([]model.WebhookEvent, error)
func (e *Engine) SendBack(ctx context.Context, tx *store.Tx, a Actor, ticketID string, to int, comment *string) ([]model.WebhookEvent, error)
func (e *Engine) Deny(ctx context.Context, tx *store.Tx, a Actor, ticketID string, comment *string) ([]model.WebhookEvent, error)
func (e *Engine) Withdraw(ctx context.Context, tx *store.Tx, a Actor, ticketID string) ([]model.WebhookEvent, error)
func (e *Engine) Archive(ctx context.Context, tx *store.Tx, a Actor, ticketID string) ([]model.WebhookEvent, error)
func (e *Engine) CanView(ctx context.Context, q store.Querier, a Actor, ticketID string) error // nil | ResourceNotFound | InvisibleTicket (INF-36)
func (e *Engine) AddComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, body string, files []string) (string, []model.WebhookEvent, error)
func (e *Engine) EditComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID, body string) ([]model.WebhookEvent, error)
func (e *Engine) DeleteComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID string) ([]model.WebhookEvent, error)
```

The `SlipFieldInput` struct must spell out the same value fields as `FieldInput` (do not embed; JSON tags must be explicit). Every engine stub body returns `kferr.New(500, "internal_server_error", "engine not implemented")`.

```go
// internal/files/files.go
type Store struct{ /* dir, key, baseURL, clock, maxBytes */ }
func New(dataDir string, key []byte, baseURL string, clock kftime.Clock, maxBytes int64) *Store
func (s *Store) Save(ctx context.Context, q store.Querier, uploaderID, filename, contentType string, r io.Reader) (model.File, error)
func (s *Store) SignedURL(f model.File) string                 // <baseURL>/files/download/<token>, 5 min
func (s *Store) DownloadHandler(db *store.DB) http.Handler       // mounted at /files/download/
func (s *Store) MaxBytes() int64
```

## Key Points and Pitfalls

- Path wildcard names must equal the spec's parameter names (`{organizationChartId}`, `{generalMasterId}`, `{itemId}`, `{signedId}`, ...), because handlers call `r.PathValue(name)`.
- `/v1/users/lookupByEmail` vs `/v1/users/{userId}` and `/v1/tickets/tasks` vs `/v1/tickets/{ticketId}` coexist: Go 1.22 ServeMux prefers the literal segment. A test must assert this.
- The catch-all must be the method-less pattern `/v1/`, so a wrong method on a known path matches it and returns 404 `endpoint_not_found` rather than 405.
- Keep `routes.go` as a plain table literal; no reflection.

## Tests (`routes_test.go`)

- `len(Operations()) == 96`; operationIds unique; (method, pattern) pairs unique; 57 distinct patterns
- the table equals a hard-coded expected list copied from the coverage matrix rows 1-96 (operationId, method, path)
- dispatch is asserted without depending on handler bodies, because wave-4 plans replace the stubs and this test must stay green. Build a mux with `Register`, then use `mux.Handler(req)`, which returns the matched pattern string: `GET /v1/users/lookupByEmail` -> pattern `GET /v1/users/lookupByEmail`; `GET /v1/tickets/tasks` -> `GET /v1/tickets/tasks`; for every `Operations()` row, a sample path (wildcards replaced by `x`) with its method -> exactly `"<METHOD> <Pattern>"`
- catch-all (served through the mux, independent of stubs): `PUT /v1/users` -> 404 `endpoint_not_found`; `GET /v1/nope` -> 404 `endpoint_not_found`
- no assertion on placeholder handler status codes or placeholder response bodies anywhere in this test file

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf10-build.log
go vet ./internal/api/... ./internal/engine/... ./internal/files/... 2>&1 | tee /tmp/pf10-vet.log
go test ./internal/api/... 2>&1 | tee /tmp/pf10-test.log
gofmt -l internal/api internal/engine internal/files   # prints nothing
```

## Done Criteria

- [ ] 96 operations registered in the 16 handler files named in the coverage matrix Handler column; engine and files contracts compile with the exact signatures; tests pass; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-10 implementation
**Intent snapshot**: Create `internal/api/v1` route/API contracts, all matrix-assigned handlers and route tests; create the engine and files contract stubs. No middleware or business behavior.
**Tasks Completed**: Added the 96-operation route table in matrix order, all 16 handler files, exact engine input/method contracts, and file store surface. Added matrix, specificity, wildcard dispatch, and catch-all route tests. Generated Go source was formatted.
**Verification**:
- `go build ./...` — exit 1, `/tmp/pf10-build.log`; compile failures are in PF-04 `internal/httpx/json.go` (`errorsAs`) and PF-08 `internal/store/read_masters.go`, `read_org.go`, `read_proxies.go`, and `read_users.go` (`valid` calls, missing `strings`, `jsonUnmarshal`).
- `go vet ./internal/api/... ./internal/engine/... ./internal/files/...` — exit 1, `/tmp/pf10-vet.log`; blocked compiling PF-08-owned store reads.
- `go test ./internal/api/...` — exit 1, `/tmp/pf10-test.log`; blocked compiling PF-08-owned store reads.
- `gofmt -l internal/api internal/engine internal/files` — exit 0, no output.
**Reconciliation requests**: PF-04 to make `errorsAs` available in `internal/httpx/json.go`; PF-08 to repair the listed store read files. After those upstream-owned paths compile on the shared tree, rerun all four verification commands and update this criterion only if every required gate passes.
