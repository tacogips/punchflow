# Round 7 UI and UI API Fixes Implementation Plan

**Status**: Completed
**Design Reference**: `design-docs/specs/design-approval-engine.md#form-inputs-and-validation`, `design-docs/specs/architecture.md#screens`
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

## Design Document Reference

Verify and fix feat:R6-12 and feat:R7-09 through feat:R7-12. Work is limited to ticket UI, fixture-backed form metadata, and punchflow UI API operations. Public REST mutations are not added where Kickflow documents no endpoint.

## Modules

### 1. Form metadata and behavior

#### `internal/model/definition.go`, `internal/present/workflow_form.go`, `web/src/components/tickets/FormFields.tsx`

```go
type GeneralMasterSearchFilter struct {
    ID string `json:"id"`
    FilterFormFieldID string `json:"filterFormFieldId"`
    FieldType string `json:"fieldType"`
    GeneralMasterFieldID *string `json:"generalMasterFieldId"`
}
```

**Checklist**:
- [x] Present configured automatic master filters and ticket-copy mappings
- [x] Apply automatic filters and selected-ticket copies in the form UI
- [x] Enforce field visibility and input flags in rendered controls and payloads

### 2. Ticket editor initialization and manual links

#### `web/src/pages/tickets/TicketEdit.tsx`, `internal/api/ticketui/`

```go
func (a *API) linkTickets(w http.ResponseWriter, r *http.Request)
func (a *API) unlinkTickets(w http.ResponseWriter, r *http.Request)
```

**Checklist**:
- [x] Ensure proxy-team resource initializes without a temporal-dead-zone error
- [x] Add reciprocal manual link/unlink UI API behavior and detail-screen controls
- [x] Add regression tests for initialization, field behavior, metadata, and linked tickets

## Module Status

| Module | File Path | Status | Tests |
|---|---|---|---|
| Form metadata and behavior | `internal/model/`, `internal/present/`, `web/src/` | COMPLETED | Presenter, OpenAPI contract, Bun |
| Editor and links | `web/src/pages/tickets/`, `internal/api/ticketui/` | COMPLETED | Bun, ticket UI API |

## Dependencies

| Feature | Depends On | Status |
|---|---|---|
| Master filter UI | Presented filter metadata and item values | COMPLETED |
| Ticket copy | Presented mapping and ticket detail API | COMPLETED |
| Manual links | Existing reciprocal store operations and visible ticket access | COMPLETED |

## Completion Criteria

- [x] Verify each finding against code and Kickflow sources
- [x] Add a regression test for every fixed finding and populated contract scenarios for shape changes
- [x] Required Go and Bun verification passes
- [x] Record inferences in `design-docs/specs/notes.md` and update affected design docs

## Progress Log

### Session: 2026-10-21
**Tasks Completed**: Fixed all five assigned findings; added Solid initialization, form behavior, populated OpenAPI contract, and reciprocal link regressions; updated design documentation and inference notes. `go build ./...`, `go vet ./...`, `go test -count=1 -race ./...` (27 packages passed, 2 had no test files), `golangci-lint run ./...` (0 issues), `bun install --frozen-lockfile`, `bun run typecheck`, `bun test` (69 passed, 0 failed), and `bun run build` all passed.
**Tasks In Progress**: None.
**Blockers**: None.
