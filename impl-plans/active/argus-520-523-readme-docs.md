# argus #520/#523: README Documentation Implementation Plan

**Plan ID**: PLAN-DOCS-README
**Status**: Ready
**Wave**: 1 (no dependencies; documents the accepted design contract, so it runs in parallel with PLAN-520-RFC3339 and PLAN-523-CREATE-WORKFLOW)
**Design Reference**: design-docs/specs/design-auth-admin.md#runtime-workflow-creation-createworkflow-argus-523 and design-docs/specs/design-api-coverage.md#timestamp-input-parsing-argus-520
**Created**: 2026-11-04
**Last Updated**: 2026-11-04

---

## Write Paths

- README.md
- impl-plans/active/argus-520-523-readme-docs.md

## Shared Paths

none

## Artifact Roots

none. This plan installs no tools, downloads nothing and generates no build output.

## Intent and context

The acceptance criteria require README.md to document the new admin endpoint and the RFC 3339 input behavior. README already has a `## Webhooks` section with an admin curl example (README.md:75-90) and a `## Point a kickflow client at punchflow` section (README.md:41).

## Non-goals

- No edits to design-docs. They are accepted and already updated.
- No Go code.
- No restructuring of other README sections.

## File-level changes

### README.md (edit)

1. In `## Point a kickflow client at punchflow`, add 1-3 sentences:
   - Date-time query filters (`*AtStart`/`*AtEnd`) accept any RFC 3339 date-time with 0-9 fractional digits and `Z` or a `+hh:mm`/`-hh:mm` offset, as well as `YYYY-MM-DD` (JST midnight).
   - Filters compare at millisecond precision, and responses always use `.000+09:00` millisecond JST.
   - Link to `design-docs/specs/design-api-coverage.md`.
2. Add a new section `## Adding a workflow at runtime`, placed after `## Webhooks` and before `## UI helper API`:
   - `POST /admin/api/workflows` takes one object in the fixture `workflows.json` element shape. `id` is optional. Referenced folders, routes and categories must already exist.
   - It needs the `workflow` permission (the seeded `pf_pat_dev_admin` has it) and does not need `--enable-admin-reset`.
   - Responses: `201` with the workflow detail; `400` invalid (the message lists the fixture validator problems); `401`/`403`; `409` for a duplicate `id` or `code`.
   - The workflow is immediately visible in `/v1/workflows` and usable for tickets. Fixture reset removes it.
   - Give one curl example in the style of README.md:80-84: `curl -X POST http://localhost:8080/admin/api/workflows -H 'Authorization: Bearer pf_pat_dev_admin' -H 'Content-Type: application/json' -d @my-workflow.json`.
   - Link to `design-docs/specs/design-auth-admin.md`.

Use plain prose with no emojis, matching the existing tone.

## Invariants

- Only README.md and this plan file change.
- Statements match the design exactly. Do not claim that GET or DELETE admin workflow endpoints exist.

## Concurrency and drift protocol

Freshly read README.md before editing and record `shasum -a 256 README.md`, before and after. If it changed unexpectedly, stop and report drift. No git write operations.

## Acceptance criteria

- [ ] The README has the RFC 3339 filter note and the `## Adding a workflow at runtime` section with the curl example.
- [ ] `grep -n "/admin/api/workflows" README.md` shows the new section.
- [ ] `grep -n "RFC 3339" README.md` shows the filter note.

## Verification (foreground; record exit status and log path)

1. `grep -n "/admin/api/workflows\|RFC 3339" README.md 2>&1 | tee /tmp/plandocs-grep.log`: at least 2 matching lines, exit 0.
2. `git diff --check -- README.md 2>&1 | tee /tmp/plandocs-check.log`: empty output, exit 0.

## Progress Log

### Session: 2026-11-04 (plan created)
**Tasks Completed**: Plan authored.
**Notes**: Implementation not started.
