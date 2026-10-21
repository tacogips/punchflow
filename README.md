# punchflow

## What punchflow is

punchflow is a local and development-only dummy for the kickflow approval workflow service. Use it to exercise an existing kickflow REST client and webhook receiver without connecting to the real service. Do not expose it to a public network. Kickflow AI features are out of scope.

## Quick start

```sh
mise install
mise run build
./punchflow serve
```

The server listens on `http://localhost:8080` and seeds the default fixtures into an empty database. To use an explicit database, run `./punchflow serve --db /tmp/pf.sqlite`. `mise run dev` is the development shortcut.

## Seeded personas

The tokens and accounts below are fixture values for local use only. They are public development credentials, not secrets; never reuse them outside a throwaway local environment.

| Persona | E-mail | Seeded role and kickflow permissions | Token |
|---|---|---|---|
| Admin (`admin`) | `admin@punchflow.test` | Super Admin: `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` | `pf_pat_dev_admin` |
| Sales manager (`sales-manager`) | `sales-manager@punchflow.test` | No seeded role permissions; approval persona | `pf_pat_dev_manager` |
| Finance manager (`finance-manager`) | `finance-manager@punchflow.test` | No seeded role permissions; approval persona | `pf_pat_dev_finance` |
| Sales employee (`sales-rep`) | `sales-rep@punchflow.test` | No seeded role permissions; requester persona | `pf_pat_dev_employee` |
| Integration service account | N/A | `integration` | `pf_sat_dev_integration` |

The other seeded roles are Ticket Admin (`ticket_read`, `ticket_write`), Master Editor (`master`, `master_read`) and Integration Admin (`integration`).

## Point a kickflow client at punchflow

Set the client's base URL to `http://localhost:8080`; keep its `/v1/...` paths. Authenticate with a seeded personal access token (PAT):

```sh
curl -H 'Authorization: Bearer pf_pat_dev_employee' \
  http://localhost:8080/v1/user
curl -H 'Authorization: Bearer pf_pat_dev_employee' \
  http://localhost:8080/v1/tickets
```

For a tenant service account token (SAT), send `X-Caller-Id` with the UUID of the user to impersonate:

```sh
curl -H 'Authorization: Bearer pf_sat_dev_integration' \
  -H 'X-Caller-Id: 00000000-0000-4000-8000-000000000011' \
  http://localhost:8080/v1/user
```

Create an `expense-claim` ticket with form field codes, then approve it by its returned ticket ID. `status` is required (`draft` or `in_progress`), and `authorTeamId` is the ID of a team the applicant belongs to; the seeded `sales-rep` belongs to `sales`. As in kickflow, `inputs` must list every form field of the workflow; send `null` for optional fields you leave empty. Set `TICKET_ID` to the `id` field in the create response before running the approve request:

```sh
curl -X POST http://localhost:8080/v1/tickets \
  -H 'Authorization: Bearer pf_pat_dev_employee' \
  -H 'Content-Type: application/json' \
  -d '{"status":"in_progress","workflowId":"1f46223b-a6ef-4ee8-884e-d3f1e6b9f13b","authorTeamId":"00000000-0000-4000-8000-000000000021","title":"Client meal","inputs":[{"formFieldCode":"amount","value":"5000"},{"formFieldCode":"purpose","value":"Client meal"},{"formFieldCode":"expense-date","value":null},{"formFieldCode":"cost-center","value":null},{"formFieldCode":"receipt","value":null},{"formFieldCode":"approver-note","value":null}]}'

curl -X POST "http://localhost:8080/v1/tickets/$TICKET_ID/approve" \
  -H 'Authorization: Bearer pf_pat_dev_manager' \
  -H 'Content-Type: application/json' -d '{}'
```

List endpoints accept `page` and `perPage` and return `Link`, `Page`, `Per-Page` and `Total` headers. Timestamps use JST with millisecond precision. Errors use `{ "code": "...", "message": "..." }`; validation errors can also include `errors`.

## Webhooks

Register an endpoint in the UI under **Admin > Webhooks**, or use `POST /admin/api/webhooks` with a webhook name, URL, optional secret and `"enabled": true` (a webhook created without `enabled` stays disabled and receives nothing):

```sh
curl -X POST http://localhost:8080/admin/api/webhooks \
  -H 'Authorization: Bearer pf_pat_dev_admin' \
  -H 'Content-Type: application/json' \
  -d '{"name":"local","url":"http://localhost:9000/hook","secret":"change-me","enabled":true}'
```

This punchflow dev admin API is separate from the kickflow-compatible `/v1` API and requires a bearer token with the `integration` permission.

The 13 event types are `ping`, `ticket_created`, `ticket_updated`, `ticket_opened`, `ticket_approved`, `ticket_confirmed`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `ticket_withdrawn`, `ticket_archived`, `comment_created` and `comment_updated`. Delivery status is `pending`, `succeeded` or `failed`.

Deliveries include `Content-Type: application/json`, `User-Agent: kickflow-Hookshot/v1`, `X-Kickflow-Delivery` and, when a secret is configured, `X-Kickflow-Signature: sha256=<hex>`. Verify the lowercase hexadecimal HMAC-SHA256 using your secret and the exact raw request body bytes. The initial delivery is followed by up to 10 retries with exponential backoff (10 seconds by default, capped at one hour). Use **Resend** in the delivery log or `POST /admin/api/webhook-events/{eventId}/resend` to retry manually; test delivery uses `POST /admin/api/webhooks/{id}/ping`. The dev admin endpoint `POST /admin/api/webhook-events` fires an event for a ticket or comment. See [webhook design](design-docs/specs/design-webhooks.md) for payload and delivery details.

## UI helper API

Browser-only helpers use `/ui/api/` with the same authentication as `/v1`. They cover application teams and workflow master-item pickers; ticket links, labels, sub-statuses, submission, approval cancellation, raising, custom steps and approver assignment; plus watch subscriptions, notifications, ticket search and CSV export. Watch, notification, search and export examples:

```sh
# Watch a ticket and read your notification inbox
curl -X POST -H 'Authorization: Bearer pf_pat_dev_manager' http://localhost:8080/ui/api/tickets/<ticket-id>/watch
curl -H 'Authorization: Bearer pf_pat_dev_manager' 'http://localhost:8080/ui/api/notifications?unread=true'

# Keyword search (ticket number, title, form values, detail rows, comments) with the usual /v1 filters
curl -H 'Authorization: Bearer pf_pat_dev_employee' 'http://localhost:8080/ui/api/tickets/search?q=lunch&status[]=in_progress'

# CSV exports (UTF-8 with BOM): ticket layout, detail-row layout and audit logs
curl -OJ -H 'Authorization: Bearer pf_pat_dev_employee' 'http://localhost:8080/ui/api/tickets/export?layout=detail'
curl -OJ -H 'Authorization: Bearer pf_pat_dev_admin' 'http://localhost:8080/ui/api/audit-logs/export'
```

Notifications are local only (no email or chat). See the [UI helper API design](design-docs/specs/design-notifications-export-search.md) and [ticket controls design](design-docs/specs/design-approval-engine.md).

## Keycloak SSO (manual check)

Start the bundled Keycloak realm, then enable OIDC by setting its issuer (or `PUNCHFLOW_OIDC_ISSUER`):

```sh
mise run keycloak:up
./punchflow serve --oidc-issuer http://localhost:8180/realms/punchflow
```

Open `http://localhost:8080`, sign in as `sales-rep@punchflow.test` with password `punchflow`, and expect the dashboard after the callback. Stop Keycloak with `mise run keycloak:down`.

## Development switches and test scenarios

Local e-mail login and the admin fixture reset API are disabled by default. Enable them with `--enable-local-login` and `--enable-admin-reset` (or their `PUNCHFLOW_ENABLE_LOCAL_LOGIN` and `PUNCHFLOW_ENABLE_ADMIN_RESET` variables). OIDC settings also accept `PUNCHFLOW_OIDC_*`; `--rate-limit N` and `--rate-limit-secret VALUE` configure `/v1` rate limiting. Tenant modes and fault injection are available through the dev admin API.

## Labels, sub-statuses and lifecycle controls

Labels (`labels.json`) and sub-statuses (`sub-statuses.json`, referenced by route steps through `subStatus`) are defined in the fixtures, like kickflow's admin-only settings. Tickets then show `labels` and `subStatus` in `/v1`, filter by `subStatusIds[]`, and a sub-status can forbid withdrawing, sending back or denying. Approval cancellation, raising to a later step and per-ticket custom steps follow the workflow's `approvalCancellable`, the step's `allowRaising` and the workflow's `allowCustomSteps`. kickflow has no public REST endpoints for labels or these controls, so the web UI uses punchflow-only endpoints under `/ui/api/` (see `design-docs/specs/design-approval-engine.md`).

## CLI, fixtures and inferred behavior

The CLI includes `serve`, `fixtures load`, `fixtures reset`, `fixtures export --out DIR`, `token issue` and `version`. Shared database options include `--data-dir`, `--db` and `--fixtures`; configuration flags have `PUNCHFLOW_...` environment forms. See the full [CLI reference](design-docs/specs/command.md). Default JSON fixtures are in `fixtures/default` (18 files, including `user-custom-fields.json`, `labels.json` and `sub-statuses.json`); export them with `punchflow fixtures export --out DIR` or restore them with `punchflow fixtures reset`. Some behavior is inferred from public kickflow materials; the inference register is in [notes.md](design-docs/specs/notes.md), and endpoint and event coverage is in the [API coverage matrix](design-docs/specs/design-api-coverage.md).

## Development

Use the uncached Go test command when checking the backend. The mise test task is a convenience task. Use bun for all web dependency, test and build commands.

```sh
go test -count=1 -v ./...
mise run test
mise run lint
cd web && bun install --frozen-lockfile && bun run build && bun test
```
