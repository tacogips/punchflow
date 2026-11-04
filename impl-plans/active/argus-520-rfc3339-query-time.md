# argus #520: RFC 3339 Query Time Input Implementation Plan

**Plan ID**: PLAN-520-RFC3339
**Status**: Ready
**Wave**: 1 (no dependencies; runs in parallel with PLAN-523-CREATE-WORKFLOW and PLAN-DOCS-README)
**Design Reference**: design-docs/specs/design-api-coverage.md#timestamp-input-parsing-argus-520
**Created**: 2026-11-04
**Last Updated**: 2026-11-04

---

## Write Paths

- internal/kftime/kftime.go
- internal/kftime/kftime_test.go
- internal/httpx/query_test.go
- internal/api/v1/tickets_rfc3339_filter_test.go
- impl-plans/active/argus-520-rfc3339-query-time.md

New files: `internal/api/v1/tickets_rfc3339_filter_test.go` does not exist yet and is created by this plan.

## Shared Paths

none

## Artifact Roots

none. This plan installs no tools, downloads nothing and generates no build output; the Go build cache stays outside the repository.

## Intent and context

`GET /v1/tickets?updatedAtStart=2026-10-28T04:08:57.101584+00:00` returns 400 today. The kickflow OpenAPI declares these filters as RFC 3339 `date-time`, so any number of fractional digits from 0 to 9 must be accepted. The cause is `internal/kftime/kftime.go:ParseInput`, which picks a layout from the string length (29, 25, 24 or 10). Its only production caller is `internal/httpx/query.go:QueryTime`, which serves every `*AtStart`/`*AtEnd` filter in `internal/api/v1/tickets.go`, `internal/api/v1/audit_logs.go` and `internal/uiapi/export.go`.

## Non-goals

- Do not change `kftime.Format`, `FormatDate`, `ToMillis` or `FromMillis`. Output stays JST with exactly 3 fractional digits.
- Do not change `internal/httpx/query.go`. `QueryTime` already maps any ParseInput error to `kferr.InvalidParameter("Invalid " + key)`.
- Do not touch body date parsing (proxies, general masters, user custom fields) or `internal/formula`.
- Do not edit README.md or design docs. PLAN-DOCS-README owns README.md, and the design is accepted.

## File-level changes

### internal/kftime/kftime.go (edit)

- Replace the length switch in `ParseInput(s string) (time.Time, error)` and keep that signature.
- Add a package-level compiled regexp, for example `rfc3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$`)`.
- Algorithm:
  1. If `len(s) == 10`, parse with `dateLayout` and return JST midnight. Keep the current behavior.
  2. Otherwise, if the regexp matches, `time.Parse(time.RFC3339Nano, s)` and return the result with full precision.
  3. Otherwise return `fmt.Errorf("unsupported time format %q", s)`.
- `timestampLayout` must stay, because `Format` still uses it. Remove the other now-unused layout literals.
- Update the doc comment to name RFC 3339 date-time plus date-only.
- Pitfalls:
  - `time.Parse` alone accepts a comma as the fraction separator and some other non-RFC 3339 forms. The regexp gate is mandatory, and it must anchor both `^` and `$`.
  - Do not truncate to milliseconds in ParseInput. The design keeps precision, and `store.Millis` truncates downstream.
  - Do not use `time.Local` for the date-only case. Keep `JST`.

### internal/kftime/kftime_test.go (edit)

Edit `TestParseInputAcceptedFormatsAndRejectsOthers`. The existing rejection cases `"2020-05-01T12:34:56.78Z"` and `"2020-05-01T12:34:56.7891+09:00"` become valid, so move them to the accepted table. Add these cases (`input -> Format(ParseInput(input))`, or `-> error`):

- `2020-05-01T12:34:56Z` -> `2020-05-01T21:34:56.000+09:00`
- `2020-05-01T12:34:56.7Z` -> `...21:34:56.700+09:00`
- `2020-05-01T12:34:56.78Z` -> `...21:34:56.780+09:00`
- `2020-05-01T12:34:56.7891+09:00` -> `...12:34:56.789+09:00`
- `2026-10-28T04:08:57.101584+00:00` -> `2026-10-28T13:08:57.101+09:00`. Also assert `got.Nanosecond() == 101584000`, so precision is kept.
- `2020-05-01T12:34:56.123456789-05:30` -> `2020-05-02T03:04:56.123+09:00`. Also assert nanoseconds are 123456789.
- Existing accepted cases stay unchanged: `.789+09:00`, no fraction `+09:00`, `.789Z`, and date-only `2020-05-01` -> `2020-05-01T00:00:00.000+09:00`.
- Rejected, each -> error: `2020/05/01`, `2020-05-01T12:34:56.789+09:00junk`, `2020-05-01t12:34:56Z`, `2020-05-01T12:34:56z`, `2020-05-01 12:34:56Z`, `2020-05-01T12:34:56`, `2020-05-01T12:34:56,789Z`, `2020-05-01T12:34:56.1234567890Z` (10 digits), `2020-05-01T12:34:56.Z`, `2020-05-01T12:34:56+0900`, `2020-13-01T00:00:00Z`, `2020-05-01T24:00:00Z`, `""`, `garbage`.
- `TestFormatUsesJSTAndTruncatesToMilliseconds` must remain unchanged and passing.

### internal/httpx/query_test.go (edit)

Add one test function, for example `TestQueryTimeRFC3339`. Follow the style of the existing `TestQueryHelpers` (httptest request with a URL query).

- `?updatedAtStart=2026-10-28T04:08:57.101584%2B00:00` -> non-nil time, no error. The `+` must be URL-encoded as `%2B` in the test URL, because a raw `+` decodes to a space.
- `?updatedAtStart=2026-10-28T04:08:57,101Z` -> error that is a `*kferr.Error` with status 400, code `invalid_parameter` and message `Invalid updatedAtStart`.
- Missing key -> `nil, nil`.

### internal/api/v1/tickets_rfc3339_filter_test.go (new)

- Package `v1_test` or whatever package the neighbouring `tickets_test.go` uses. Imitate `internal/api/v1/tickets_test.go`, which calls `env.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?...AtStart=...", nil)` around line 200.
- Test `TestListTicketsAcceptsRFC3339MicrosecondFilter`:
  - `testutil.New(t)`; `env.Do(t, testutil.Admin, GET, "/v1/tickets?updatedAtStart=2026-10-28T04:08:57.101584%2B00:00", nil)` -> status 200 with a JSON array body.
  - The same path with `updatedAtStart=2026-10-28T04:08:57.101Z` -> 200.
  - `updatedAtStart=not-a-time` -> `testutil.AssertError(t, r, 400, "invalid_parameter")`.

## Invariants

- `kftime.Format` output is byte-identical for every instant.
- Date-only input still means JST midnight.
- Invalid input still produces `400 invalid_parameter` `Invalid <key>` through QueryTime.
- No other files are modified.

## Concurrency and drift protocol

Other wave-1 workers share the working tree (PLAN-523 edits `internal/admin/*`; PLAN-DOCS edits README.md). Before each edit, freshly read the target file and record `shasum -a 256 <file>`. After the edit, record the hash again. If a file you own changed unexpectedly between your read and your write, stop and report drift instead of overwriting. Touch only the four files above and this plan file. Do not run git commit, git stash, git checkout or any other git write operation.

## Acceptance criteria

- [ ] ParseInput uses the regexp gate plus `time.RFC3339Nano`, and date-only still means JST midnight.
- [ ] All kftime test cases listed above exist and pass.
- [ ] The QueryTime 400 test exists and passes.
- [ ] The `/v1/tickets` microsecond filter test returns 200.
- [ ] `gofmt -l` reports nothing for the owned files.

## Verification (each run in the foreground; record exit status and log path)

1. `gofmt -l internal/kftime internal/httpx/query_test.go internal/api/v1/tickets_rfc3339_filter_test.go 2>&1 | tee /tmp/plan520-gofmt.log`: empty output, exit 0.
2. `go test -count=1 ./internal/kftime/... ./internal/httpx/... 2>&1 | tee /tmp/plan520-unit.log`: `ok` for both packages, exit 0.
3. `go test -count=1 ./internal/api/v1/ -run 'TestListTicketsAcceptsRFC3339MicrosecondFilter|Ticket|AuditLog' 2>&1 | tee /tmp/plan520-api.log`: `ok`, exit 0. This shows existing ticket and audit filters did not regress.
4. `go vet ./internal/kftime/... ./internal/httpx/... ./internal/api/v1/ 2>&1 | tee /tmp/plan520-vet.log`: exit 0.

Workspace-wide `go build ./... && go vet ./... && go test ./...` belongs to reconcile and integration review.

## Progress Log

### Session: 2026-11-04 (plan created)
**Tasks Completed**: Plan authored from the accepted design.
**Notes**: Implementation not started.
