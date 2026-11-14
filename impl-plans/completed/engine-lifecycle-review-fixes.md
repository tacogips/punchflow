# Ticket Lifecycle Review Fixes Implementation Plan

**Status**: Completed
**Design Reference**: [design-approval-engine.md#state-machine](../../design-docs/specs/design-approval-engine.md#state-machine), [design-approval-engine.md#runtime-lifecycle-controls-pf-19](../../design-docs/specs/design-approval-engine.md#runtime-lifecycle-controls-pf-19), [design-webhooks.md#event-emission-matrix](../../design-docs/specs/design-webhooks.md#event-emission-matrix), [notes.md](../../design-docs/specs/notes.md)
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Design Document Reference

**Sources**: `design-docs/specs/design-approval-engine.md`, `design-docs/specs/design-webhooks.md`, `design-docs/specs/notes.md`; relevant Kickflow manuals and `design-docs/references/kickflow-openapi.yaml` as named below.

### Summary

Correct ticket lifecycle edge cases found in the second-round review. Keep the engine transaction and outbox model intact, align design rules and inference records with verified manual/OpenAPI behavior, and add regression tests for each accepted finding.

### Scope

**Included**: WH-08, WH-07/NF-06, NF-07, NF-05, NF-08, NF-09, NF-13, and remaining PF-13 (tenant fiscal year start month).

**Excluded**: Findings owned by the parallel API, webhook, and feature worktrees; unrelated engine changes.

## Modules

### 1. Terminal lifecycle timestamps and sub-status

#### `internal/engine/actions.go`, `internal/engine/substatus.go`, `internal/engine/user_cleanup.go`

**Status**: COMPLETED

**Checklist**:
- [x] Set `closedAt` to the completion or archive time, including approval/confirmation completion, immediate completion on submit, explicit archive, and user-deletion archive; verify denial and withdrawal leave it null.
- [x] Verify withdrawn behavior against `TicketSummary.closedAt` in `design-docs/references/kickflow-openapi.yaml` and encode the result in regression tests and design.
- [x] Preserve the current sub-status when the entering group configures none; restore the manual-defined sub-status on send-back; clear it on completion, denial, withdrawal, archive, and user-deletion archive.
- [x] Add regression coverage for each terminal path, send-back, and entering a group without a sub-status.

### 2. Raising, cancellation, and outbox behavior

#### `internal/engine/controls*.go`, `internal/engine/routing.go`

**Status**: COMPLETED

**Checklist**:
- [x] Make Raise advance exactly one group per operation; a later-step assignment grants eligibility only. Permit the action types documented for raising and retain its audit log.
- [x] Remove webhook events from Raise; revise INF-65/INF-66 and the `design-webhooks.md` event matrix. Keep `ticket_updated` for other documented control mutations as appropriate.
- [x] Allow approval cancellation by the assignee or active proxy when the current group is the same as or immediately follows the acted-on group, provided nobody in that following group approved; reject when two or more groups later.
- [x] Check cancellation eligibility using the assignment and active proxy relationship, not only `ActualApproverID`.
- [x] Add control regression cases and an HTTP-level outbox assertion proving Raise creates no webhook event while audit logging remains.

### 3. Fallback route materialization

#### `internal/engine/routing.go`, route/step engine tests

**Status**: COMPLETED

**Checklist**:
- [x] For `no_fallback` with no assignee, materialize an empty waiting step and keep the ticket `in_progress`, unassigned, instead of returning 422.
- [x] Ensure a zero-assignee step cannot auto-complete, including within parallel groups.
- [x] Cover submission and parallel-group progression with regression tests; revise INF-19 and the lifecycle design.

### 4. Effective proxy validation and submission form checks

#### `internal/engine/update.go`, forms submission checks

**Status**: COMPLETED

**Checklist**:
- [x] Revalidate the effective proxy client (stored value or newly supplied value) on draft/rejected saves and submission; suspended or deleted clients return 422 `validation_failed`.
- [x] Preserve explicit-null semantics so `proxyClientUserId: null` removes the proxy client.
- [x] At submission, run radio-button and other submission-only checks against every final stored detail row, including rows/cells omitted from the incoming patch.
- [x] Add regression tests for stored/new/cleared proxy values and retained detail-row cells. Use the 2026-10-20 API changelog for proxy status behavior.

### 5. Tenant-configured fiscal numbering

#### `internal/engine/numbering.go`, tenant fixture/model/loading

**Status**: COMPLETED

**Checklist**:
- [x] Read fiscal-year start month from tenant configuration instead of hardcoding April.
- [x] Add a tenant fixture field with an April default and thread it through the model and fixture load/export paths used by numbering.
- [x] Add regression tests for default April and a non-April tenant start month.

### 6. Design and inference synchronization

#### `design-docs/specs/design-approval-engine.md`, `design-docs/specs/design-webhooks.md`, `design-docs/specs/notes.md`

**Status**: COMPLETED

**Checklist**:
- [x] Update the state machine, timestamp and terminal sub-status rules, fallback behavior, proxy validation, submission checks, numbering configuration, and runtime controls to match implemented behavior.
- [x] Correct the webhook event matrix for Raise and other affected operations.
- [x] Update INF-60, INF-62, INF-65, INF-66, and INF-19; record clarified inferences under unique `INF-ENG-` IDs with manual/OpenAPI/changelog sources.

## Module Status

| Module | File Path | Status | Tests |
|---|---|---|---|
| Terminal lifecycle and sub-status | `internal/engine/actions.go`, `substatus.go`, `user_cleanup.go` | COMPLETED | Lifecycle regressions pass |
| Raise and cancellation | `internal/engine/controls*.go`, `routing.go` | COMPLETED | Control and HTTP outbox regressions pass |
| Fallback progression | `internal/engine/routing.go` | COMPLETED | Submission/parallel regressions pass |
| Proxy and form validation | `internal/engine/update.go`, form submission code | COMPLETED | Save/submission regressions pass |
| Fiscal-year numbering | `internal/engine/numbering.go`, tenant fixture/model | COMPLETED | Default and custom-month regressions pass |
| Design and inference records | `design-docs/specs/*.md` | COMPLETED | Reviewed against source evidence, code, and tests |

## Dependencies

| Feature | Depends On | Status |
|---|---|---|
| Terminal timestamps and sub-status | Existing lifecycle transitions | Available |
| Raise/cancel controls | Group progression and active proxy lookup | Available; coordinate shared control files |
| Fallback progression | Route build and advancement semantics | Available |
| Proxy/form validation | Stored update state and submission normalization | Available |
| Fiscal-year numbering | Tenant fixture/model access | Available |
| Design synchronization | Accepted implementation and manual/OpenAPI evidence | Follows implementation |
| Full verification and commit | All implementation and documentation tasks | Pending |

## Completion Criteria

- [x] Each accepted finding is verified against the current code and has a regression test; no requested finding was skipped.
- [x] HTTP-level outbox assertions cover Raise and its audit log.
- [x] Design documents and `INF-ENG-` inference records match implementation.
- [x] `go build ./...` passes.
- [x] `go vet ./...` passes.
- [x] `go test -count=1 -race ./...` passes (26 packages passed; 0 failures).
- [x] `golangci-lint run ./...` reports 0 issues.
- [x] Commit all work in this worktree using the required structured message; do not push or add AI attribution.

## Progress Log

### Session: 2026-10-21 12:10 JST
**Tasks Completed**: Implemented all eight requested lifecycle fixes, added regression tests, updated design/inference records, and passed full verification.
**Tasks In Progress**: None.
**Blockers**: None.
**Notes**: Verified the cited Kickflow help-center articles through the Zendesk JSON API, the 2026-10-20 developer changelog entry, and the checked-in OpenAPI `closedAt` definition. Full repository build, vet, 26-package race suite, and lint passed.

## Related Plans

- **Previous**: None
- **Next**: None
- **Depends On**: None
