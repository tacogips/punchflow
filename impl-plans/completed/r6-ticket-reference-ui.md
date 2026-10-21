# Round 6 Ticket Reference and UI Fixes Implementation Plan

**Status**: Completed
**Design Reference**: `design-docs/specs/design-approval-engine.md#form-inputs-and-validation`, `design-docs/specs/architecture.md`
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

## Design Document Reference

**Source**: `design-docs/specs/design-approval-engine.md`

### Summary

Represent and enforce ticket-reference workflow restrictions, present accurate defaults and reference choices in workflow forms, and make UI helper selections follow the effective applicant and item validity dates.

### Scope

**Included**: R6-11 through R6-14, regression coverage, fixture validation, and design-note updates.
**Excluded**: REST endpoints not documented by kickflow, database migrations, and unrelated review findings.

## Modules

### 1. Ticket field configuration and validation

#### `internal/model/definition.go`, `internal/engine/forms.go`

```go
type FormField struct {
    SelectableTicketWorkflowCodes []string `json:"selectableTicketWorkflowCodes,omitempty"`
}

type SlipField struct {
    SelectableTicketWorkflowCodes []string `json:"selectableTicketWorkflowCodes,omitempty"`
}

func EngTicketWorkflowID(ctx context.Context, q Querier, ticketID string) (string, bool, error)
```

**Checklist**:
- [x] Parse fixture workflow-code restrictions and validate workflow references
- [x] Present resolved `selectableTicketWorkflows` on form and detail fields
- [x] Reject new ticket references whose workflow is outside a non-empty restriction
- [x] Validate against OpenAPI with populated form and detail field scenarios

### 2. Application UI helpers and defaults

#### `internal/api/ticketui/ticketui.go`, `web/src/pages/tickets/TicketEdit.tsx`, `web/src/components/tickets/SlipTable.tsx`

```go
func (a *API) listApplicationTeams(w http.ResponseWriter, r *http.Request)
func (a *API) listWorkflowMasterItems(w http.ResponseWriter, r *http.Request)
```

```ts
export function defaultFieldValue(field: FormField): unknown
```

**Checklist**:
- [x] Load proxy-client teams only for an authorized workflow registration
- [x] Hide master items outside their validity dates in the ticket picker
- [x] Resolve detail-field default master items and initialize new detail rows from field defaults
- [x] Cover proxy entitlement, expired item filtering, and defaults with regressions

## Module Status

| Module | File Path | Status | Tests |
|--------|-----------|--------|-------|
| Ticket field configuration and validation | `internal/model/`, `internal/fixtures/`, `internal/engine/`, `internal/present/` | COMPLETED | engine and OpenAPI contract |
| Application UI helpers and defaults | `internal/api/ticketui/`, `web/src/` | COMPLETED | ticket UI API and Bun tests |

## Dependencies

| Feature | Depends On | Status |
|---------|------------|--------|
| Presenter and engine ticket workflow choices | Workflow fixture model and store lookup | COMPLETED |
| Proxy team helper | Existing proxy-applicant registration validation | COMPLETED |
| Master item picker validity | Application clock and general-master item date fields | COMPLETED |

## Completion Criteria

- [x] Verify findings against source code and kickflow references
- [x] Add regression tests for each fixed item and populated OpenAPI response shapes
- [x] `go build ./...`, `go vet ./...`, `go test -count=1 -race ./...`, and `golangci-lint run ./...` pass
- [x] `bun install --frozen-lockfile`, `bun run typecheck`, `bun test`, and `bun run build` pass
- [x] Record inferences and update affected design documents

## Progress Log

### Session: 2026-10-21
**Tasks Completed**: Verified and fixed R6-11 through R6-14; added engine, picker, presenter, web and OpenAPI regressions; updated design notes and documentation.
**Tasks In Progress**: None.
**Blockers**: None.
**Notes**: No migration was needed. Go build, vet and golangci-lint passed; the full race suite passed (27 packages passed, 2 had no test files, 0 failures). Bun typecheck, 64 tests, and production build passed.
