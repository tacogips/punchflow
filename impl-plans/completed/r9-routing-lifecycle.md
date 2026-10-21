# Round 9 Routing and Lifecycle Fixes

**Status**: Completed
**Design Reference**: design-docs/specs/design-approval-engine.md#engine-surface
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Design Document Reference

**Source**: design-docs/specs/design-approval-engine.md

### Summary
Restore previous-approver nomination through the punchflow UI API, correct lifecycle and assignment edge cases, and align grade-restricted sharing and custom-step placement with Kickflow manuals.

### Scope
**Included**: feat:R9-01, feat:R9-02, feat:R8-04/wh:WH-32, wh:WH-31, feat:R7-04, feat:R9-07, feat:R9-03; tests and related design notes.
**Excluded**: Other findings in the round-9 reports, public `/v1` endpoint changes, schema migrations unless unavoidable.

## Modules

### 1. Routing and lifecycle behavior

#### internal/engine and internal/api/ticketui

```go
func (e *Engine) NominateNextApprovers(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, in ApproverInput) ([]model.WebhookEvent, error)
func (e *Engine) AddCustomStep(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, in CustomStepInput) ([]model.WebhookEvent, error)
```

**Checklist**:
- [ ] Add current-approver nomination, preserving applicant nomination restrictions.
- [ ] Count distinct assignees and preserve approval thresholds during handover.
- [ ] Correct proxy-authored applicant transfers, per-step self-approval checks, timestamps, and custom-step send-back dates.
- [ ] Enforce custom-step insertion positions.

### 2. Sharing and UI

#### internal/engine/visibility.go, internal/access/section.go, web/src/pages/tickets

```go
func pf14UserInTeam(ctx context.Context, q store.Querier, userID, teamID string, descendants bool, minimumGradeID string) (bool, error)
```

**Checklist**:
- [ ] Apply minimum-grade semantics to ticket and section sharing.
- [ ] Expose nomination in the ticket UI using `/ui/api`.

## Module Status

| Module | File Path | Status | Tests |
|---|---|---|---|
| Routing and lifecycle | `internal/engine`, `internal/api/ticketui` | COMPLETED | Regression and HTTP tests passed |
| Sharing and UI | `internal/engine`, `internal/access`, `web/src` | COMPLETED | Access and frontend tests passed |
| Design notes | `design-docs/specs` | COMPLETED | Manuals and OpenAPI reviewed |

## Dependencies

| Feature | Depends On | Status |
|---|---|---|
| Routing, lifecycle, sharing | Existing engine and UI API | Available |
| Design notes | Verified implementation and manuals | In progress |

## Completion Criteria

- [x] Every supported finding has a regression test; REST PATCH wording is corrected to match OpenAPI and the UI-only nomination path is documented.
- [x] `go build ./...`, `go vet ./...`, `go test -count=1 -race ./...`, and `golangci-lint run ./...` pass.
- [x] `bun install --frozen-lockfile`, `bun run typecheck`, `bun test` (72 pass, 0 fail), and `bun run build` pass.
- [x] Design notes and touched design docs are updated.

## Progress Log

### Session: 2026-10-21
**Tasks Completed**: All seven assigned fixes, regression tests, and design-note updates.
**Tasks In Progress**: None.
**Blockers**: None.
**Notes**: `go build ./...`, `go vet ./...`, and `golangci-lint run ./...` exited 0 (0 lint issues). `go test -count=1 -race ./...` exited 0 with 27 tested packages passing and 2 packages without test files. Frozen-lockfile Bun install, typecheck, 72 Bun tests, and production build passed. No migration was required.

