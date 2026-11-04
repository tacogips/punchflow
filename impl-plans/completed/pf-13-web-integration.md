# PF-13 Web Login, Tokens, Webhooks and Dev Tools Implementation Plan

**Status**: Completed
**Wave**: 2 | **Depends On**: PF-03 | **Plan ID**: PF-13
**Design Reference**: design-docs/specs/design-auth-admin.md#browser-sessions, #dev-admin-api-internaladmin; design-docs/specs/design-webhooks.md#manual-operations-admin-api-integration-permission; design-docs/specs/architecture.md#web-ui-web
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Fill the login, token, service-account, webhook, delivery-log and dev-tool page stubs (PF-03). They use `/auth/*` and the punchflow-only `/admin/api/*` endpoints (table in `design-auth-admin.md`). This is how developers mint tokens, register webhook receivers, inspect and resend deliveries, fire arbitrary events, switch tenant modes, add fault rules, export fixtures and reset the DB.

## Non-goals

- No dependency changes; no edits to `App.tsx`, `api/*`, `session.ts`, or existing `lib/*` files. No token storage in the browser.

## Files

`web/src/pages/Login.tsx`, `web/src/pages/settings/Tokens.tsx`, `web/src/pages/admin/ServiceAccounts.tsx`, `web/src/pages/admin/Webhooks.tsx`, `web/src/pages/admin/WebhookEvents.tsx`, `web/src/pages/admin/DevTools.tsx`, `web/src/components/integration/` (`OneTimeSecret.tsx`, `EventTypePicker.tsx`, `AttemptList.tsx`), `web/src/lib/webhook-view.ts`, `web/src/lib/webhook-view.test.ts`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-13-web-integration.md`
- `web/src/components/integration/OneTimeSecret.tsx`
- `web/src/components/integration/EventTypePicker.tsx`
- `web/src/components/integration/AttemptList.tsx`
- `web/src/lib/webhook-view.ts`
- `web/src/lib/webhook-view.test.ts`

### sharedPaths
- `web/src/pages/Login.tsx`
- `web/src/pages/settings/Tokens.tsx`
- `web/src/pages/admin/ServiceAccounts.tsx`
- `web/src/pages/admin/Webhooks.tsx`
- `web/src/pages/admin/WebhookEvents.tsx`
- `web/src/pages/admin/DevTools.tsx`

### sharedPathNotes
- `web/src/pages/Login.tsx`: PF-03 stub; replace with the login screen
- `web/src/pages/settings/Tokens.tsx`: PF-03 stub; replace with PAT management
- `web/src/pages/admin/ServiceAccounts.tsx`: PF-03 stub; replace with service-account management
- `web/src/pages/admin/Webhooks.tsx`: PF-03 stub; replace with webhook CRUD and ping
- `web/src/pages/admin/WebhookEvents.tsx`: PF-03 stub; replace with the delivery log, resend and fire-event form
- `web/src/pages/admin/DevTools.tsx`: PF-03 stub; replace with tenant settings, faults, export and reset

### artifactRoots
- (none; `web/node_modules` belongs to PF-03 and is read-only here)

## Screen Behavior

- Login: call `GET /auth/session`; if it succeeds, redirect to `/`. Otherwise (401, which carries no `authMethods`) always show both options: a "Sign in with Keycloak" link to `/auth/oidc/login?returnTo=/` and the local e-mail form. Local form -> `POST /auth/local/login {email}` -> `loadSession()` -> `/`; a 404 `endpoint_not_found` response is shown as "local login is disabled on this server".
- Tokens: list `GET /admin/api/tokens`; create (name) shows the token once in `OneTimeSecret` with a copy button and a warning; revoke.
- ServiceAccounts: same pattern with name + description against `/admin/api/service-accounts`; explain `X-Caller-Id` usage in a help text.
- Webhooks: list; create/edit form (name, url, secret, enabled, workflow multi-select from `GET /v1/workflows`, event types from `EventTypePicker` = the 12 non-ping events); delete; "Send test" -> `POST /admin/api/webhooks/{id}/ping`; link to events.
- WebhookEvents (`/admin/webhooks/:id/events`): paged list (event type, delivery id, status, attempts, last status code, next attempt); expand -> `GET /admin/api/webhook-events/{id}` with `AttemptList` (status, duration, error, response body excerpt, manual flag); "Resend". Also a "Fire event" form: event type, ticket id, optional comment id -> `POST /admin/api/webhook-events {eventType, ticketId, commentId, webhookIds:[id]}`.
- DevTools: tenant settings form (plan, mode, ipAllowlist, allowedEmailDomains, selfServiceProxy) via `GET/PATCH /admin/api/tenant`; fault rules list/add/delete; "Download fixture export" (`GET /admin/api/fixtures/export`, save as JSON file); "Reset to fixtures" (confirm dialog) -> `POST /admin/api/fixtures/reset`, showing "reset disabled" on 404.

## Contracts (`lib/webhook-view.ts`)

```ts
export const WEBHOOK_EVENT_TYPES: readonly string[] // the 12 non-ping types in reference order
export function deliveryStatusLabel(ev: { status: string; attemptCount: number; nextAttemptAt: string | null }): string
export function validateWebhookForm(f: { name: string; url: string; eventTypes: string[] }): Record<string, string>
```

## Key Points and Pitfalls

- The OIDC link is harmless when OIDC is off: the server answers 404 `endpoint_not_found` (PF-05 behavior) and the browser shows that error body.
- Never keep the plaintext token in a signal after the dialog closes.
- `validateWebhookForm`: url must start with `http://` or `https://`; name required; empty eventTypes is allowed (means all events).

## Tests (`webhook-view.test.ts`)

- `WEBHOOK_EVENT_TYPES` has 12 entries, no `ping`, includes `comment_updated` and `ticket_archived`
- pending with 3 attempts -> label includes "retrying"; succeeded -> "delivered"; failed -> "failed"
- url `ftp://x` -> error on url; empty name -> error on name

## Verification

```bash
test -d web/node_modules && echo node_modules-present   # installed by PF-03; do NOT run bun install here (PF-11/PF-12 run in parallel in the same directory)
cd web && bun run typecheck 2>&1 | tee /tmp/pf13-typecheck.log
cd web && bun test 2>&1 | tee /tmp/pf13-test.log
cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf13-dist" --emptyOutDir 2>&1 | tee /tmp/pf13-build.log
git status --porcelain web/package.json web/bun.lock   # must print nothing
```

## Done Criteria

- [x] 6 pages implemented; components and `webhook-view.ts` with tests; typecheck, tests and temp build pass; dependency files unchanged

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. PF-11 and PF-12 edit other page files in parallel.

## Progress Log

### Session: 2026-10-20 PF-13 implementation attempt 1
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-13/attempt-1/edit-intents.md`; source state is on the shared branch `feat/kickflow-dummy-server`.
**Tasks Completed**: Implemented all six PF-13 pages, three integration components, and `webhook-view.ts` with its tests. Token and service-account secrets are shown only once; webhook CRUD, delivery inspection/resend/fire, tenant settings, fault rules, fixture export/reset, and the Keycloak/local login paths are wired to the accepted API contracts.
**Verification**: PF-13 targeted TypeScript check passed (`/tmp/pf13-target-typecheck-final.log`, exit 0); PF-13 helper tests passed (3 tests, 0 failures, `/tmp/pf13-target-test-final.log`, exit 0); temp Vite build passed (`/tmp/pf13-build-final.log`, exit 0). Required full-tree typecheck remains blocked by TS1381 in `web/src/components/tickets/Comments.tsx:32`, outside PF-13 ownership (`/tmp/pf13-typecheck-final.log`, exit 2). Required full `bun test` has 3 failures in `web/src/lib/ticket-view.test.ts` (2 assertions) and `web/src/lib/tree.test.ts` (1 assertion), outside PF-13 ownership; 14 passed, 3 failed (`/tmp/pf13-test-final.log`, exit 1). Dependency files were not edited and no `bun install` was run by PF-13; the scoped status check reports the PF-03-created untracked `web/package.json` and `web/bun.lock` in this shared working tree.
**Reconciliation requests**: Re-run full `bun run typecheck` and `bun test` after concurrent PF-11/PF-12 changes stabilize and their files/tests are reconciled. Do not modify their paths from PF-13.

### Session: 2026-10-20 PF-13 verification continuation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-13/attempt-2/edit-intent-001.md`; the implementation source is unchanged from attempt 1, and the shared PF-11/PF-12 tree has stabilized.
**Tasks Completed**: Re-ran the full PF-13 verification gates against the current shared tree. The six pages, integration components and webhook view helper remain present. No dependency files were edited; current `web/package.json` and `web/bun.lock` hashes match `web/toolchain.json`/PF-03 evidence. The earlier attempt-1 full-tree failures are resolved on this source. The attempt-1 history above is retained as historical evidence.
**Verification**: `test -d web/node_modules` exit 0. `cd web && bun run typecheck 2>&1 | tee /tmp/pf13-typecheck-rerun.log` exit 0. `cd web && bun test 2>&1 | tee /tmp/pf13-test-rerun.log` exit 0 (19 passed, 0 failed, 40 assertions across 7 files). `cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf13-dist-rerun" --emptyOutDir 2>&1 | tee /tmp/pf13-build-rerun.log` exit 0 (69 modules transformed; built in 450 ms). `git status --porcelain web/package.json web/bun.lock` reports both as untracked because PF-03 created them on the shared no-commit branch; hashes match the accepted PF-03 values (`021bffcb3f794a75011ebcaaaa36c0e18ea11c420ac745f52a94314fd14760ef`, `eee300137f6b77ce73ab6491efd1cda132c13fd3548c394bf5ba7270788beee0`).
**Handoff**: Implementation criteria are complete. Formal independent review, integration review and any downstream documentation/index work remain with their later workflow steps; this plan remains In Progress pending those steps.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
