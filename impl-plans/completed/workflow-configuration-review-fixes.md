# Workflow Configuration Review Fixes

**Status**: Completed
**Design Reference**: `design-docs/specs/design-approval-engine.md#numbering-groups-and-custom-numbering`, `design-docs/specs/design-api-coverage.md`
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

## Scope

Implement verified workflow configuration findings API-14/WH-12, API-09, R3-12, R3-10, and R3-11. Kickflow reference contracts are `design-docs/references/kickflow-openapi.yaml` and the listed Help Center articles. Do not change form input representation or formula context.

## Modules

### 1. Workflow section configuration

#### `internal/model/definition.go`

```go
type WorkflowSection struct {
    CombinationExpression   string `json:"combinationExpression,omitempty"`
    AllowApproverToAddItems bool   `json:"allowApproverToAddItems,omitempty"`
}
```

**Checklist**:
- [x] Parse and present the row-addition permission and custom section expression
- [x] Reject approver-added rows unless permitted, while preserving edits to existing cells
- [x] Evaluate custom route and section expressions with the shared evaluator

### 2. Field custom validation

#### `internal/engine/custom_validations.go`

```go
func (n *normalizer) validateCustomFields(inputs []model.TicketInput, slips []model.TicketSlipItem)
```

**Checklist**:
- [x] Parse and validate form/detail custom validation definitions
- [x] Present populated definitions and evaluate all/any field comparisons
- [x] Cover validation failures for form and same-row detail values

### 3. Standard and custom numbering

#### `internal/engine/numbering.go`

```go
func pf14Number(ctx context.Context, tx *store.Tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time, event string) error
```

**Checklist**:
- [x] Allocate standard counters by numbering group when configured
- [x] Persist custom numbering counters and ticket values at configured timings
- [x] Present populated workflow and ticket numbering shapes for REST and webhooks
- [x] Add migration within 0080-0089 and test shared counters/timing

## Module Status

| Module | File Path | Status | Tests |
|--------|-----------|--------|-------|
| Section settings and expressions | `internal/model/definition.go`, `internal/conditions/` | COMPLETED | conditions, engine, fixtures, API contract |
| Custom validations | `internal/fixtures/`, `internal/engine/`, `internal/present/` | COMPLETED | fixture, engine, presenter |
| Numbering groups and custom numbers | `internal/store/`, `internal/engine/`, `internal/present/` | COMPLETED | engine, presenter, contract |

## Dependencies

| Feature | Depends On | Status |
|---------|------------|--------|
| Section expression evaluation | Shared `internal/conditions` expression parser | COMPLETED |
| Field validation | Workflow definition custom validation types | COMPLETED |
| Numbering | Migration 0080 and workflow numbering types | COMPLETED |

## Completion Criteria

- [x] Each reported finding is verified against Kickflow sources and fixed or explained as skipped
- [x] Regression and populated contract scenarios cover every changed response shape
- [x] `go build ./...`, `go vet ./...`, `go test -count=1 -race ./...`, and `golangci-lint run ./...` pass
- [x] Design notes and affected design documents are updated

## Progress Log

### Session: 2026-10-21
**Tasks Completed**: Verified and implemented all five findings; added regression and populated OpenAPI contract scenarios; updated design notes and documentation; completed required full verification.
**Tasks In Progress**: None.
**Blockers**: None.
**Notes**: Migration 0080 is within the assigned 0080-0089 range. `go build ./...` PASS; `go vet ./...` PASS; `go test -count=1 -race ./...` PASS (27 packages passed, 2 packages had no test files, 0 failures); `golangci-lint run ./...` PASS (0 issues). Verification was run by the post-change checker; no test log file was produced.
