# PF-04 httpx Implementation Plan

**Status**: Ready
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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf04-build.log
go vet ./internal/httpx/... 2>&1 | tee /tmp/pf04-vet.log
go test -race ./internal/httpx/... 2>&1 | tee /tmp/pf04-test.log
gofmt -l internal/httpx   # prints nothing
```

## Done Criteria

- [ ] Every contract symbol exists; every listed test passes with `-race`; gofmt clean; files < 1000 lines

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
