# Round 4 Routing, Conditions, and Nominations Implementation Plan

**Status**: Completed
**Design Reference**: [design-approval-engine.md](../../design-docs/specs/design-approval-engine.md#route-resolution-on-submit-and-re-submit); [notes.md](../../design-docs/specs/notes.md#routing-and-nomination-compatibility-inf-rte-)
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

## Intent and Scope

Repair the verified Round-4 routing, condition, nomination, eligibility, and proxy lookup defects so REST and webhook behavior matches kickflow. Preserve `/v1` contracts; no UI-only endpoint is needed. `feat:R3-04` is included because the reviewed base did not share the same step-entry operation between advancement and send-back.

## Modules

### 1. Route condition resolution

#### `internal/engine/routing.go`

```go
func pf14RouteStepValues(def model.WorkflowDef, values map[string]any) map[string]any
func pf14bStepConditionMatch(ctx context.Context, tx store.Querier, ticket *model.Ticket, fields []model.ConditionField, combination string, values map[string]any) (bool, error)
```

**Checklist**:
- [x] Treat restricted form references as known-empty in both condition modes.
- [x] Resolve applicant-team codes and General Master selection metadata.
- [x] Support documented `other_variable` conditions in fixtures and execution.

### 2. Nomination eligibility and counts

#### `internal/engine/submit.go`, `internal/engine/update.go`, `internal/engine/routing.go`, `internal/engine/actions.go`

```go
func pf14bValidateNominee(ctx context.Context, q store.Querier, userID string, targets []model.RouteTarget, values map[string]any) error
func pf14CheckNextCustomApprovers(ctx context.Context, tx *store.Tx, ticket model.Ticket) error
```

**Checklist**:
- [x] Require active users and enforce route team/grade restrictions across nomination paths.
- [x] Enforce `minCustomAssignees` for previous-step nominations, including zero-skip.

### 3. Step entry and application eligibility

#### `internal/engine/actions.go`, `internal/engine/create.go`, `internal/engine/submit.go`, `internal/engine/routing.go`

```go
func (e *Engine) pf14EnterStep(ctx context.Context, tx *store.Tx, ticket *model.Ticket, step *model.TicketStep, now time.Time) error
func pf14CheckApplicable(ctx context.Context, q store.Querier, userID string, def model.WorkflowDef, authorTeamID string) error
```

**Checklist**:
- [x] Share team assignment refresh and viewer synchronization between advancement and send-back.
- [x] Apply manager self-approval as the final fallback after searches and configured fallback.
- [x] Reject approval-only application teams on create and submission.

### 4. Proxy applicant lookup

#### `internal/store/engine_refs.go`

```go
func EngActiveProxyApplicant(ctx context.Context, q Querier, clientUserID, proxyUserID, workflowID, date string) (bool, error)
```

**Checklist**:
- [x] Treat an empty workflow restriction list as unrestricted while retaining date checks.

## Status Tracking

| Module | File Path | Status | Tests |
|---|---|---|---|
| Route condition resolution | `internal/engine/routing.go`, `internal/fixtures/validate.go` | Implemented | Restricted values, team/master references, other variable |
| Nomination eligibility and counts | Engine routing/actions/submit/update | Implemented | Draft, submission, customization, previous-step minimum |
| Step entry and application eligibility | Engine actions/create/submit | Implemented | Send-back, manager fallback, approval-only teams |
| Proxy applicant lookup | `internal/store/engine_refs.go` | Implemented | Unrestricted proxy create |

## Dependencies

| Feature | Depends On | Status |
|---|---|---|
| Nomination checks | Route targets and active membership lookups | Complete |
| Final verification | All implementation modules | Complete |

## Completion Criteria

- [x] Add a regression for each verified finding.
- [x] Append `INF-RTE-01` through `INF-RTE-08` and update the approval-engine design.
- [x] `go build ./...` and `go vet ./...` pass.
- [x] `go test -count=1 -race ./...` passes with zero failures.
- [x] `golangci-lint run ./...` reports zero issues.
- [x] No touched Go file is at least 1000 lines.

## Progress Log

### Session: 2026-10-21

**Tasks Completed**: Findings verified against the review reports, code, and official Help Center JSON. Implementation and regression tests added for all eight requested items.
**Tasks In Progress**: None.
**Blockers**: None.
**Notes**: `feat:R3-04` was confirmed as a real defect by comparing the clean base with the working diff; `pf14EnterStep` now serves both advancement and send-back.

**Final Verification**:
- `go build ./...`: exit 0.
- `go vet ./...`: exit 0.
- `go test -count=1 -race ./...`: exit 0; 27 packages passed, 0 failed, 0 skipped; 2 packages had no test files.
- `golangci-lint run ./...`: exit 0; 0 issues.
- `git diff --check`: exit 0.
- The largest touched Go source file is below 1000 lines.
