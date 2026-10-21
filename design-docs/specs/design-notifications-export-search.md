# Watch Subscriptions, Notification Inbox, CSV Export and Keyword Search

Design of three punchflow UI/dev features that close basic-feature gaps against kickflow (review items PF-20, PF-21, PF-22): per-user ticket watch subscriptions with a local notification inbox, CSV export of ticket search results and audit logs, and free-text ticket search.

## Overview

kickflow's public REST API documents none of these features, so they are deliberately **not** part of `/v1`. They live in a punchflow-only namespace, `/ui/api/`, served by `internal/uiapi` and authenticated by the same middleware as `/v1` (Bearer token, or session cookie plus the `X-Punchflow-CSRF` header on `POST`/`DELETE`). The `/v1` contract, its operation inventory and the coverage test are untouched.

| Feature | Review item | kickflow manual | Packages |
|---|---|---|---|
| Watch subscriptions and notification inbox | PF-20 | Watch (360045069513) | `internal/notify`, `internal/store` (watches, notifications), `internal/uiapi` |
| CSV export (tickets, detail rows, audit logs) | PF-21 | Ticket CSV (1500007064762), audit log (360045335414) | `internal/export`, `internal/uiapi` |
| Keyword search | PF-22 | Search (40767576061081) | `internal/ticketsearch`, `internal/store` (`ticket_search.go`), `internal/uiapi` |

| Surface | Path prefix | Auth |
|---|---|---|
| UI helper API | `/ui/api/` | Same as `/v1` |

### Route table (`internal/uiapi`)

| Method | Path | Purpose |
|---|---|---|
| GET | `/ui/api/tickets/{ticketId}/watch` | Caller's watch state for a ticket |
| POST | `/ui/api/tickets/{ticketId}/watch` | Watch the ticket |
| DELETE | `/ui/api/tickets/{ticketId}/watch` | Stop watching (also opts out of a workflow-level watch) |
| GET | `/ui/api/notifications` | Caller's inbox, newest first; `unread=true`, `page`, `perPage` |
| GET | `/ui/api/notifications/unread-count` | `{count}` |
| POST | `/ui/api/notifications/{id}/read` | Mark one read; returns `{count}` |
| POST | `/ui/api/notifications/read-all` | Mark all read; returns `{count}` |
| GET | `/ui/api/tickets/search` | `/v1/tickets` filters plus `q`; same JSON and pagination headers as `GET /v1/tickets` |
| GET | `/ui/api/tickets/export` | CSV of the complete filtered result; `layout=ticket` (default) or `detail`; same filters plus `q` |
| GET | `/ui/api/audit-logs/export` | CSV of all audit logs matching `userId`, `createdAtStart`, `createdAtEnd`, `sortBy`; needs the `audit` permission |

State-changing routes use `POST`/`DELETE` (never `PUT`) because the shared middleware only applies its CSRF check to `POST`, `PATCH` and `DELETE`. Unknown paths under `/ui/api/` return the kickflow-style `404 endpoint_not_found`.

## Technical Details

### Storage

Migration `0030_watch_notifications.sql`:

| Table | Columns | Notes |
|---|---|---|
| `ticket_watches` | `ticket_id`, `user_id`, `state` (`watching` \| `unwatched`), `created_at`, `updated_at` | Primary key `(ticket_id, user_id)`. A row is an explicit choice. No row means the default applies. Cascades with the ticket and the user. |
| `notifications` | `id`, `user_id`, `kind`, `ticket_id`, `comment_id`, `actor_user_id`, `excerpt`, `dedupe_key`, `read_at`, `created_at` | Unique index `(user_id, dedupe_key)` where `dedupe_key` is not null. Cascades with the ticket and the user; the actor is set to null when the actor is deleted. |

`TruncateAll` clears both tables through the foreign-key cascades of `tickets` and `users`, so `truncateOrder` is unchanged.

### Watching

Effective watchers of a ticket are:

1. workflow-level watchers from the workflow definition (`watchers`: users, and the members of watcher teams in the active org chart) who have not opted out, then
2. users with an explicit `watching` row.

A workflow-level watcher is therefore subscribed automatically, without any row, and the watch toggle reports `source: "workflow"`. Calling `DELETE` records `unwatched`, which wins over the workflow definition from then on; `POST` records `watching` again (`source` stays `workflow` when the definition also lists the user). The workflow is resolved when events fire, so watchers added to a workflow later also receive notifications for tickets that already exist.

Watching requires that the caller can view the ticket (`403 invisible_ticket` otherwise, `404 resource_not_found` for an unknown ticket). Watching does not grant access.

Watch state response: `{ticketId, watching, source: "" | "manual" | "workflow", workflowWatcher}`.

### Notification fan-out

Notifications are produced inside the transaction that produced the event, without changing the engine or the `/v1` handlers. `server.New` wraps the webhook service in `notify.Sink`, which implements the `Events` interface of `internal/api/v1` (`Enqueue`, `Notify`). `Sink.Enqueue` first enqueues the webhook deliveries and then calls `notify.Service.Fanout` with the same events and the same transaction. A failure in either rolls the request back. Events created by the admin webhook test routes (`EnqueueTo`) never reach the sink and produce no notifications.

| Engine event | Notification kind | Recipients |
|---|---|---|
| `ticket_opened` | `ticket_applied` | author, proxy client, watchers |
| `ticket_approved` | `ticket_approved` | author, proxy client, watchers |
| `ticket_confirmed` | `ticket_confirmed` | author, proxy client, watchers |
| `ticket_rejected` (send back, reject to author) | `ticket_rejected` | author, proxy client, watchers |
| `ticket_denied` | `ticket_denied` | author, proxy client, watchers |
| `ticket_completed` | `ticket_completed` | author, proxy client, watchers |
| `ticket_withdrawn` | `ticket_withdrawn` | author, proxy client, watchers |
| `ticket_archived` | `ticket_archived` | author, proxy client, watchers |
| `ticket_opened`, `ticket_approved`, `ticket_confirmed`, `ticket_rejected` while `in_progress` | `approval_requested` | unfinished assignees of the current actionable steps |
| `comment_created` | `comment_mention` | users mentioned in the comment |
| `comment_created` | `comment_created` | author, proxy client, watchers, except mentioned users |
| `ticket_created`, `ticket_updated`, `comment_updated`, `ping` | none | |

Rules applied to every recipient:

- The acting user is skipped (a user is not notified of their own action), except for `approval_requested`, which is a task: an actor who is also the next approver still receives it.
- The recipient must be an `activated` user and must be able to view the ticket (`Engine.CanView`, the single effective-visibility rule of INF-36). A notification never discloses a ticket the user cannot open.
- One notification per user per event and kind. A user who is mentioned does not also receive `comment_created` for the same comment.
- `approval_requested` is idempotent per assignment: `dedupe_key = approval:<ticket_assignee id>:<assignee updated_at ms>`. Replaying an event, or a partial approval that leaves other assignees untouched, does not repeat a request. Sending a ticket back re-activates the steps (their assignee rows are updated), so the approvers are asked again.
- `excerpt` holds the first 140 characters of a comment with `<@user-id>` markup replaced by `@<last name> <first name>`; it is empty for lifecycle kinds. The ticket number and title are joined when the inbox is read, so they reflect the current ticket.

Proxy approvers are not notified separately: the original assignee receives the request and proxies see the task through the task list, as in the rest of punchflow.

No email, chat or push delivery exists (DIV-02). The inbox is the only observable output.

### Inbox API

`GET /ui/api/notifications` returns an array of
`{id, kind, ticketId, ticketNumber, ticketTitle, ticketVisible, commentId, actor: {id, name} | null, excerpt, read, readAt, createdAt}` with the standard `Page`, `Per-Page`, `Total` and `Link` headers. `ticketNumber` and `ticketTitle` are `null` and `ticketVisible` is `false` when the user can no longer view the ticket. Marking another user's notification read returns `404 resource_not_found`.

### Keyword search

`GET /ui/api/tickets/search?q=<keyword>&<any GET /v1/tickets filter>` returns the same array and pagination headers as `GET /v1/tickets`.

Semantics:

- The keyword is split on whitespace (full-width spaces included) into at most 8 terms of at most 100 characters. **Every term** must match (AND). A blank keyword applies no keyword restriction.
- A term matches, as a case-insensitive substring (SQLite `LIKE`, so ASCII case folding only; `%` and `_` are literal), any of: the ticket number, the title, a form input value, a detail (slip) row cell value, or the body of a comment that is not deleted. Different terms may match different sources.
- Form input values are matched as stored: text, numbers and array elements. References (master items, users, teams, tickets) are matched by their stored identifiers, not by their display names. Keys of detail-row cells are never matched.
- **Visibility.** The structured filters and effective ticket visibility are obtained by calling the `GET /v1/tickets` handler in-process with every parameter except `q`, `page`, `perPage` and `layout`, paging through its complete result. Search and export therefore always agree with the ticket list, including filters added to `/v1/tickets` later, and a `/v1` validation error (for example an invalid `status[]`) is returned unchanged. The keyword matches are then intersected with that result and re-paginated. At most 10 000 tickets are collected; a larger result returns `400 invalid_parameter` asking to narrow the filters.
- **Section view restrictions.** Candidates come from SQL over all stored values. For tickets whose workflow has restricted form sections the caller may not view (`access.SectionAccess`; `ticket_read` administrators see everything), the keyword is re-evaluated in Go against only the visible text, so a value in a hidden section cannot be matched or inferred. Titles, numbers, comments and detail rows are not section-restricted.

### CSV export

`GET /ui/api/tickets/export` collects the complete result exactly as the search does (filters, `q` and visibility; `page` and `perPage` are ignored) and renders it:

| Layout | Rows | Fixed columns | Dynamic columns |
|---|---|---|---|
| `ticket` (default) | one per ticket | Ticket ID, Ticket number, Title, Workflow, Status, Author, Author email, Author team, Created at, Opened at, Completed at, Archived at, Updated at | one per form field of the workflows in the result, titled by the field title, in workflow and section order |
| `detail` | one per detail (slip) row | Ticket ID, Ticket number, Title, Workflow, Status, Author, Section, Row no. | one per detail field of the workflows in the result |

- Columns are keyed by field code, so workflows sharing a field code share one column; the header is the title of the first field seen. A workflow without detail rows contributes no rows to the `detail` layout.
- Content is rendered from the per-caller ticket presentation (`present.TicketDetailFor`), so inputs and detail rows of restricted sections the caller cannot view are empty or absent.
- References are written by name (master item name, user full name, team full name, ticket number or title, attachment file names); arrays are joined with `, `; numbers are written without exponent. Timestamps use the API format (JST, millisecond precision).
- `audit-logs/export` writes: Created at, Action, User ID, User name, User email, Ticket ID, System type, Remote IP, Request ID, Log ID, Data (the JSON object as text). Order follows `sortBy=createdAt[-asc|-desc]`, newest first by default.

File format (identical for every export): UTF-8 with a byte order mark, RFC 4180 quoting, CRLF line endings (also inside quoted cells, as `encoding/csv` writes them), `Content-Type: text/csv; charset=utf-8`, `Content-Disposition: attachment; filename="<tickets-ticket|tickets-detail|audit-logs>-<yyyymmdd-hhmmss JST>.csv"`, `Cache-Control: no-store`. The body is built in memory first so a failure never produces a truncated download.

Spreadsheet formula injection is neutralised: a cell beginning with `=`, `+`, `@`, tab or carriage return, or with `-` unless the whole cell is a plain number, is prefixed with an apostrophe.

### Web UI

| Screen | Change |
|---|---|
| Ticket detail | `Watch` / `Unwatch` toggle in the header (`WatchToggle`) |
| Sidebar | `Notifications` link with an unread badge (`UnreadBadge`, refreshed every 30 s and on navigation) |
| `/notifications` | Inbox page: unread-only filter, mark one or all as read, link to the ticket, pagination |
| Ticket search | Keyword box; with a keyword the page calls `/ui/api/tickets/search`, otherwise `/v1/tickets`; "Ticket layout" and "Detail row layout" CSV links that carry the current filters and keyword |
| Audit logs | "Export CSV" link carrying the current filters |

Pure logic is in `web/src/lib/notification-view.ts` and `web/src/lib/ticket-search.ts` with bun tests.

## Usage Examples

```bash
# Watch a ticket, then read the inbox (token auth shown; the UI uses its session)
curl -X POST   -H "Authorization: Bearer $TOKEN" localhost:8080/ui/api/tickets/$ID/watch
curl           -H "Authorization: Bearer $TOKEN" "localhost:8080/ui/api/notifications?unread=true"

# Keyword search and export of the same result
curl -H "Authorization: Bearer $TOKEN" "localhost:8080/ui/api/tickets/search?q=offsite+lunch&status[]=in_progress"
curl -OJ -H "Authorization: Bearer $TOKEN" "localhost:8080/ui/api/tickets/export?q=offsite&layout=detail"
curl -OJ -H "Authorization: Bearer $TOKEN" "localhost:8080/ui/api/audit-logs/export?userId=$USER_ID"
```

## Limitations

- Workflow-level watchers who cannot view a ticket are not notified (kickflow's rule for whether watchers also gain access is not documented).
- Free-text search matches stored values only (no reference names, no full-width/half-width or kana normalisation, ASCII-only case folding in SQL).
- Edited comments do not notify newly added mentions; only `comment_created` is fanned out.
- The in-process `/v1` list is read page by page, so search and export cost grows with the number of visible tickets (fine for the dev dataset, capped at 10 000).

## References

- `design-docs/specs/notes.md` (INF-NES-01 .. INF-NES-14)
- `design-docs/specs/design-auth-admin.md` (authentication, CSRF)
- `design-docs/specs/design-webhooks.md` (the outbox the sink decorates)
- kickflow manuals: watch https://support.kickflow.com/hc/ja/articles/360045069513, ticket CSV https://support.kickflow.com/hc/ja/articles/1500007064762, audit log https://support.kickflow.com/hc/ja/articles/360045335414, search https://support.kickflow.com/hc/ja/articles/40767576061081
