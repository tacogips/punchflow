# PF-04 httpx Implementation Plan

**Status**: Completed
**Wave**: 2 | **Depends On**: PF-01 | **Plan ID**: PF-04
**Design Reference**: design-docs/specs/design-api-coverage.md#cross-cutting-v1-conventions, #error-code-catalog; design-docs/specs/notes.md INF-01, INF-02, INF-03, INF-07, INF-32, INF-33, INF-38, INF-39; design-docs/specs/design-auth-admin.md (faults)
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

`internal/httpx` holds the kickflow wire conventions shared by `/v1` and `/admin/api`: JSON encode/decode, pagination, sorting, array queries, request id, panic recovery, CORS, the 411 rule, rate limiting and fault injection. Errors are `*kferr.Error` from PF-01 and are written with `kferr.Write`.

## Non-goals

- No authentication (PF-05), no routing (PF-10/PF-16), no store access.
- Do not change `internal/kferr` (log a reconciliation request if a constructor is missing).

## Files

`internal/httpx/json.go`, `pagination.go`, `query.go`, `middleware.go`, `ratelimit.go`, `faults.go`, and a `_test.go` per file.

## Path Declarations

### writePaths
- `impl-plans/active/pf-04-httpx.md`
- `internal/httpx/json.go`
- `internal/httpx/pagination.go`
- `internal/httpx/query.go`
- `internal/httpx/middleware.go`
- `internal/httpx/ratelimit.go`
- `internal/httpx/faults.go`
- `internal/httpx/json_test.go`
- `internal/httpx/pagination_test.go`
- `internal/httpx/query_test.go`
- `internal/httpx/middleware_test.go`
- `internal/httpx/ratelimit_test.go`
- `internal/httpx/faults_test.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

```go
func WriteJSON(w http.ResponseWriter, status int, v any)   // json.Marshal, no trailing newline, charset=utf-8; nil slice -> "[]" is the caller's job
func WriteEmpty(w http.ResponseWriter)                       // 200, no body, no Content-Type
func DecodeJSON(r *http.Request, v any) error                // *kferr.Error on failure
const MaxJSONBody = 10 << 20

type Page struct{ Page, PerPage int }
func ParsePage(r *http.Request) (Page, error)               // defaults 1/25; INF-03
func (p Page) Offset() int
func SetPagination(w http.ResponseWriter, r *http.Request, baseURL string, p Page, total int) // Link, Page, Per-Page, Total

type Sort struct{ Field string; Desc bool; Set bool }
func ParseSort(r *http.Request, allowed ...string) Sort      // INF-02: invalid/unsupported -> Set=false
func QueryArray(r *http.Request, key string, allowBare bool) ([]string, error)
func QueryTime(r *http.Request, key string) (*time.Time, error)   // kftime.ParseInput; error -> invalid_parameter
func QueryString(r *http.Request, key string, maxLen int) (string, error)

func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler // first mw is outermost
func RequestID(next http.Handler) http.Handler; func RequestIDFrom(ctx context.Context) string
func Recover(next http.Handler) http.Handler
func CORS(next http.Handler) http.Handler
func LengthRequired(next http.Handler) http.Handler
func ClientIP(r *http.Request) string                         // RemoteAddr host only

type RateLimiter struct{ /* fixed 60s window per IP */ }
func NewRateLimiter(limit int, secret string, clock kftime.Clock) *RateLimiter // limit 0 -> disabled
func (l *RateLimiter) Middleware(next http.Handler) http.Handler

type FaultRule struct {
    ID, Method, PathPrefix, Code, Message string
    Status, Remaining int // Remaining 0 = unlimited
    CreatedAt time.Time
}
type FaultSet struct{ /* mutex-guarded, in memory */ }
func NewFaultSet() *FaultSet
func (f *FaultSet) Add(r FaultRule) FaultRule      // assigns ID (crypto/rand UUID), CreatedAt
func (f *FaultSet) List() []FaultRule; func (f *FaultSet) Delete(id string) bool; func (f *FaultSet) Clear()
func (f *FaultSet) Middleware(next http.Handler) http.Handler
```

## Key Points and Pitfalls

- `DecodeJSON`: if `Content-Length != 0` or the body is chunked, `Content-Type` must be `application/json` (parameters allowed) -> else `InvalidContentType`. Empty body decodes to the zero value (body-less POST is valid). Use `http.MaxBytesReader` -> `PayloadTooLarge`. `*json.SyntaxError` / unexpected EOF -> `InvalidJSON`. `*json.UnmarshalTypeError` -> `InvalidParameter("expected <type>, but received <type>")` with JSON type names (`array`, `String`, `Integer`, ...; INF-38). Do not call `DisallowUnknownFields`.
- `ParsePage`: `perPage` 0 or 101 or `abc` -> `InvalidParameter`; `page=0` -> `InvalidParameter`.
- `SetPagination`: absolute URLs `baseURL + r.URL.Path` with every original query param kept and `page`/`perPage` replaced; relations in order `first, prev, next, last`, each only when applicable (INF-01); `last` page = max(1, ceil(total/perPage)). Separator `, `.
- `QueryArray(key)`: reads `key[]` values. When `allowBare` is true, a single bare `key=v` is accepted. A bare key repeated, or present at all when `allowBare` is false -> `InvalidParameter("Invalid request: value at root is not an array")`.
- `CORS`: on every response set `Access-Control-Allow-Origin: *` and `Access-Control-Expose-Headers` (Link, Page, Per-Page, Total, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset, X-Request-Id). `OPTIONS` with `Access-Control-Request-Method` -> 200 immediately with allow-methods `GET, POST, PATCH, DELETE` and allow-headers `Authorization, Content-Type, X-Caller-Id, X-Rate-Limit-Secret`; never call `next`.
- `LengthRequired`: only when `r.ProtoMajor == 1` and method is POST or PUT: no `Content-Length` header in `r.Header` and `len(r.TransferEncoding) == 0` -> status 411, empty body. Note: Go's server keeps the `Content-Length` header in `r.Header`; test with a raw TCP request, because `httptest.NewRequest` and the Go client always set the length.
- `Recover`: log the panic with slog (including request id), respond `kferr.Internal()`. Must not swallow `http.ErrAbortHandler` (re-panic it).
- `RateLimiter` (INF-07): disabled -> no headers at all. Enabled: key = ClientIP; `X-Rate-Limit-Secret == secret` (non-empty) -> limit x10, counted in a separate bucket. Set `RateLimit-Limit`, `RateLimit-Remaining` (never negative), `RateLimit-Reset` (window end, Unix seconds) before calling next; when exceeded -> 429 `RateLimited()` with the same headers. Windows start at the first request (fixed 60 s) and use the injected clock.
- `FaultSet.Middleware`: the first matching rule (method empty = any; `strings.HasPrefix(r.URL.Path, PathPrefix)`) writes `kferr.New(Status, Code, Message or default "fault injected")`. Decrement `Remaining`; remove the rule when it reaches 0 after use. Must be race-free (`go test -race`).
- `RequestID`: new UUID per request (crypto/rand), set the `X-Request-Id` response header, store it in the context.

## Tests (input -> expected)

- POST JSON with `Content-Type: text/plain` -> 400 `invalid_content_type`; `{"a":` -> 400 `invalid_json`; `{"to":"x"}` into `struct{To int}` -> 400 `invalid_parameter` with "expected"; 11 MiB body -> 400 `payload_too_large`; empty body + no content type -> ok
- `?perPage=101` -> 400; no params -> Page{1,25}
- total 120, perPage 25, page 2 -> `Link` has first, prev, next, last (last page=5), `Total: 120`; page 1 -> no first/prev; page 5 -> no next/last
- `?status[]=a&status[]=b` -> [a b]; `?status=a` with allowBare -> [a]; `?status=a&status=b` -> 400
- `sortBy=createdAt-desc` allowed [createdAt] -> {createdAt, true, true}; `sortBy=name` -> Set=false
- raw TCP `POST /x HTTP/1.1` without length -> 411; with `Content-Length: 0` -> passes
- OPTIONS preflight -> 200 with CORS headers, next not called
- limit 2: 3rd request -> 429 `rate_limited`, `RateLimit-Remaining: 0`; advance fake clock 61 s -> allowed; with secret -> limit 20
- fault rule {pathPrefix:"/v1/tickets", status:504, code:"gateway_timeout", remaining:1} -> first request 504, second passes
- panic in handler -> 500 `internal_server_error`, `X-Request-Id` set

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 2. **Depends on**: PF-02 (phase 1; it changes `internal/model`, which this package does not import, so the dependency is only for wave ordering).

### Why

The implementation was complete at 410d3d8, but session-460 evidence mixed cached runs with uncached ones and was never accepted. This session only re-verifies with the session-462 evidence rule (`impl-plans/README.md` rule 11). No source change is expected.

### Non-goals

- Do not edit any file unless an uncached test fails. In that case, fix only PF-04 files, recording pre/post hashes. Do not touch `internal/kferr` (accepted PF-01).

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf04-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf04-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf04-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf04-s462-test.log)"
gofmt -l internal/httpx   # prints nothing
wc -l internal/httpx/*.go | sort -n | tail -2   # largest < 1000
```

Evidence: exit 0 for each command; FAIL=0, SKIP=0, PASS >= 19 (the session-460 uncached count).

### Done Criteria (session 462)

- [x] Uncached `-race -v` httpx tests pass, with counts recorded; build and vet exit 0; gofmt clean; no source change, or each change justified with hashes

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-04 implementation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-04/attempt-1/intent-001.md` records implementation of the six httpx helpers and tests; `intent-002.md` records this plan update; `intent-003.md` records formatting and the test assertion correction; `intent-004.md` records the explicit zero-time rate bucket; `intent-005.md` records this progress update; `intent-007.md` records final verification and completion tracking.
**Tasks Completed**: Added JSON encode/decode and request limits, pagination/sort helpers, query parsing, middleware, IP fixed-window rate limiting, fault injection, and six focused test files. All PF-04 source files are below 1000 lines.
**Verification**:
- `go build ./... 2>&1 | tee /tmp/pf04-build.log` — exit 1. Current shared tree has errors in PF-02-owned `internal/store/read_masters.go`, `internal/store/read_proxies.go`, and `internal/store/webhooks.go`; no PF-04 file is named in the failures.
- `go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-vet.log` — exit 0.
- `go test -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-test.log` — exit 0.
- `go test -race -v ./internal/httpx/... 2>&1 | tee /tmp/pf04-test-verbose.log` — exit 0; 11 top-level tests passed, including 7 JSON subtests.
- `gofmt -l internal/httpx` (captured in `/tmp/pf04-gofmt.log`) — exit 0, no output.
- `wc -l internal/httpx/*.go` — largest file `pagination.go`, 110 lines.
- SHA-256 hashes of the 12 implementation/test files are recorded in `tmp/punchflow-kickflow-dummy-s460/PF-04/attempt-1/final-source-sha256.txt`.
- Final combined-tree reruns: `go build ./... 2>&1 | tee /tmp/pf04-final-build.log` — exit 0; `go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-final-vet.log` — exit 0; `go test -race -count=1 -v ./internal/httpx/... 2>&1 | tee /tmp/pf04-final-test.log` — exit 0, 11 top-level tests passed; `gofmt -l internal/httpx` — exit 0, no output. Explicit `EXIT_STATUS=0` is appended to each log.
- Independent post-modification check also passed build, vet, race tests, and gofmt; logs: `/tmp/pf04-postcheck-build.log`, `/tmp/pf04-postcheck-vet.log`, `/tmp/pf04-postcheck-test.log`, `/tmp/pf04-postcheck-gofmt.log`.
**Reconciliation requests**: The earlier moving-tree build failure listed PF-02-owned `internal/store/read_masters.go`, `internal/store/read_proxies.go`, and `internal/store/webhooks.go`; final combined-tree build now passes, so no PF-04 reconciliation request remains. Formal independent review and downstream final integration remain in later workflow steps.

Implementation Done Criteria are complete. PF-27 owns final combined-tree reconciliation beyond these current passing gates and plan archiving.

### Session: 2026-10-20 PF-04 Step 6 revalidation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-04/attempt-2/intent-001.md` records appending this revalidation session to this plan only.
**Tasks Completed**: Rechecked PF-04 against its accepted design and contract. The pre-step fanout snapshot matches all 13 declared PF-04 files byte-for-byte; the read-only contract audit found no mismatches. No Go source or test changes were needed. All source files remain below 1000 lines; the largest is `pagination.go` at 110 lines.
**Verification** (current source fingerprint unchanged across the four gates; full logs end with explicit exit status):
- `go build ./... 2>&1 | tee /tmp/pf04-build.log` — exit 0; `/tmp/pf04-build.log`.
- `go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-vet.log` — exit 0; `/tmp/pf04-vet.log`.
- `go test -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-test.log` — exit 0; `/tmp/pf04-test.log`; Go reported the package result as cached and did not provide test/assertion counts.
- `go test -race -count=1 -v ./internal/httpx/...` — exit 0; `/tmp/pf04-test-verbose.log`; 19 top-level/subtest assertions passed, 0 failed. An initial output capture is retained at `/tmp/pf04-test-verbose-attempt1.log`; zsh rejected the wrapper's read-only variable name after the test run, so that incomplete capture is superseded by the complete log with explicit `EXIT_STATUS=0`.
- `gofmt -l internal/httpx` — exit 0 with no files listed; `/tmp/pf04-gofmt.log`.
- Shared-tree fingerprint before and after verification: 483 files, SHA-256 `cdb4b2c62e5ada5c0eab024a9f9821774995e45cdf4472ab345a151f10888fe3`.
**Reconciliation requests**: None. Formal independent review and downstream final integration remain owned by later workflow steps.

### Session: 2026-10-20 PF-04 Step 6 current-source revalidation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-04/attempt-3/intent-001.md` records this plan-only progress append and its exact intended content.
**Tasks Completed**: Confirmed PF-01 is accepted for this plan, compared all 13 PF-04 files against `fanoutChangeEvidencePath` (all SHA-256 hashes match), and rechecked the assigned contract. No Go source or test files changed. The largest PF-04 Go file remains `pagination.go` at 110 lines.
**Verification** (corrected reruns are complete and include `EXIT_STATUS=0`):
- `go build ./... 2>&1 | tee /tmp/pf04-step6-build-rerun.log` — exit 0.
- `go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-step6-vet-rerun.log` — exit 0.
- `go test -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-step6-test-rerun.log` — exit 0; package output was cached, so no test counts were reported.
- `go test -race -count=1 -v ./internal/httpx/... 2>&1 | tee /tmp/pf04-step6-test-verbose-rerun.log` — exit 0; 19 top-level/subtest assertions passed and 0 failed.
- `gofmt -l internal/httpx 2>&1 | tee /tmp/pf04-step6-gofmt-rerun.log` — exit 0; no files listed.
- Initial wrapper attempts at `/tmp/pf04-step6-build.log`, `/tmp/pf04-step6-vet.log`, `/tmp/pf04-step6-test.log`, `/tmp/pf04-step6-test-verbose.log`, and `/tmp/pf04-step6-gofmt.log` ended with zsh's `status` read-only-variable error after invoking the checks. These incomplete captures are superseded by the corrected reruns above; no underlying check failed.
**Reconciliation requests**: None. Formal independent review, final shared-tree integration, and plan archiving remain owned by later workflow steps.

### Session: 2026-10-20 PF-04 Step 6 current-source verification
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s462/PF-04/attempt-1/edit-intent-001.md` records this plan-only progress update and completion-criteria change.
**Tasks Completed**: Reverified the assigned PF-04 contract on the current shared tree. No PF-04 Go source or test files changed. The largest PF-04 Go file remains `pagination.go` at 110 lines.
**Verification** (complete logs include explicit exit status):
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf04-s462-build.log; status=${PIPESTATUS[0]}; echo EXIT_STATUS=$status >> /tmp/pf04-s462-build.log; exit $status'` — exit 0; `/tmp/pf04-s462-build.log`.
- `bash -o pipefail -c 'go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-s462-vet.log; status=${PIPESTATUS[0]}; echo EXIT_STATUS=$status >> /tmp/pf04-s462-vet.log; exit $status'` — exit 0; `/tmp/pf04-s462-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-s462-test.log; status=${PIPESTATUS[0]}; echo EXIT_STATUS=$status >> /tmp/pf04-s462-test.log; exit $status'` — exit 0; `/tmp/pf04-s462-test.log`; PASS=19, FAIL=0, SKIP=0.
- `echo "PASS=$(grep -c -- '--- PASS' /tmp/pf04-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf04-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf04-s462-test.log)` — PASS=19, FAIL=0, SKIP=0.
- `gofmt -l internal/httpx` — exit 0; no files listed.
- `wc -l internal/httpx/*.go | sort -n | tail -2` — exit 0; largest Go file is `pagination.go` at 110 lines.
- Independent repository post-check repeated build, scoped vet, uncached race tests and gofmt; `/tmp/pf04-s462-*-review.log`; exit 0 and tests PASS=19, FAIL=0, SKIP=0.
- SHA-256 hashes for all PF-04 Go source and test files are recorded in `tmp/punchflow-kickflow-dummy-s462/PF-04/attempt-1/final-source-sha256.txt`.
**Reconciliation requests**: None. Formal independent review and downstream final integration remain owned by later workflow steps.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
