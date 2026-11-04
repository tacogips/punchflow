# PF-01 Foundation Primitives Implementation Plan

**Status**: Completed
**Wave**: 1 | **Depends On**: - | **Plan ID**: PF-01
**Design Reference**: design-docs/specs/architecture.md#package-layout, #developer-tooling; design-docs/specs/design-api-coverage.md#cross-cutting-v1-conventions; design-docs/specs/notes.md INF-33
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

punchflow is a dev-only dummy kickflow server (Go module `punchflow`, go 1.25.4). The repo is an empty scaffold: `cmd/punchflow/.gitkeep`, `internal/.gitkeep`, `internal/build/VERSION` (contains `0.1.0`), `mise.toml`, `.gitignore`. This plan creates the stdlib-only primitives every other Go plan imports (time formatting, kickflow error type, version, CLI entry, embed anchors) and the tooling files (mise tasks, gitignore).

## Non-goals

- No external Go dependencies (PF-02 owns `go.mod`/`go.sum`).
- No `serve`/`fixtures`/`token` logic (PF-16 replaces the CLI stubs).
- No HTTP middleware (PF-04).
- Do not touch `scripts/`, `packaging/`, `.claude/`, `design-docs/`.

## Files

| Path | Action |
|---|---|
| `internal/kftime/kftime.go`, `internal/kftime/fake.go`, `internal/kftime/kftime_test.go` | create |
| `internal/kferr/kferr.go`, `internal/kferr/kferr_test.go` | create |
| `internal/build/version.go`, `internal/build/version_test.go` | create (keep `VERSION` unchanged) |
| `internal/cli/cli.go`, `internal/cli/cli_test.go` | create (skeleton) |
| `cmd/punchflow/main.go` | create; delete `cmd/punchflow/.gitkeep` (`rm` the file; no git command) |
| `embed.go` (module root, `package punchflow`), `embed_test.go` | create |
| `web/dist/.gitkeep`, `fixtures/default/.gitkeep` | create (empty files) |
| `mise.toml`, `.gitignore` | modify |

## Path Declarations

### writePaths
- `impl-plans/active/pf-01-foundation-primitives.md`
- `internal/kftime/kftime.go`
- `internal/kftime/fake.go`
- `internal/kftime/kftime_test.go`
- `internal/kferr/kferr.go`
- `internal/kferr/kferr_test.go`
- `internal/build/version.go`
- `internal/build/version_test.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `cmd/punchflow/main.go`
- `cmd/punchflow/.gitkeep`
- `embed.go`
- `embed_test.go`
- `web/dist/.gitkeep`
- `fixtures/default/.gitkeep`
- `mise.toml`
- `.gitignore`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts (pin exactly; other plans compile against these)

```go
// internal/kftime
type Clock interface{ Now() time.Time }
type SystemClock struct{}
var JST *time.Location // time.FixedZone("JST", 9*60*60); never time.LoadLocation
func Format(t time.Time) string              // "2006-01-02T15:04:05.000+09:00" in JST
func FormatDate(t time.Time) string          // "2006-01-02" in JST
func ParseInput(s string) (time.Time, error) // the 4 accepted formats; date-only = 00:00 JST
func ToMillis(t time.Time) int64
func FromMillis(ms int64) time.Time
type FakeClock struct{ /* mutex-guarded */ }
func NewFakeClock(t time.Time) *FakeClock
func (c *FakeClock) Now() time.Time; func (c *FakeClock) Set(t time.Time); func (c *FakeClock) Advance(d time.Duration)

// internal/kferr
type Error struct {
    Status  int
    Code    string
    Message string
    Errors  map[string][]string // only for 422 validation errors; nil otherwise
}
func (e *Error) Error() string
func New(status int, code, message string) *Error
func As(err error) (*Error, bool)
func Write(w http.ResponseWriter, err error) // non-*Error -> 500 internal_server_error
```

`kferr` also exports one constructor per documented code (names are the contract): `InvalidParameter(msg)`, `InvalidContentType()`, `InvalidJSON()`, `PayloadTooLarge()`, `InvalidHeader(msg)`, `InvalidAccessToken()`, `UserNotRegistered()`, `UserInactive()`, `InvalidCallerID()`, `InvitationExpired()`, `MissingPermission(msg)`, `UpgradeRequired()`, `InvisibleTicket()`, `UserNotAssigned()`, `TicketCreateRestricted()`, `IPAddressDenied()`, `AdminIsWorking()`, `TenantSuspended()`, `TenantMaintenance()`, `ResourceNotEditable()`, `ResourceNotFound()`, `EndpointNotFound()`, `ValidationFailed(fieldErrors map[string][]string)` (message = first message joined, e.g. `"amount must not be empty"`), `InvalidState(msg)`, `DomainNotAllowed()`, `AssignmentRequiredBeforeApprove()`, `InputRequiredBeforeApprove()`, `RateLimited()`, `Internal()`. HTTP statuses and codes exactly as in the error catalog of `design-api-coverage.md`.

```go
// internal/build
func Version() string // //go:embed VERSION, strings.TrimSpace

// internal/cli
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int

// embed.go (package punchflow)
//go:embed all:web/dist
var WebDist embed.FS
//go:embed all:fixtures/default
var DefaultFixtures embed.FS
```

## Key Points and Pitfalls

- `kferr.Write` body: `{"code":..,"message":..}` plus `"errors"` only when non-nil; never `doc_url`. Use `json.Marshal` (HTML escaping on, no trailing newline; INF-33). Set `Content-Type: application/json; charset=utf-8` before `WriteHeader`. Do NOT use `json.NewEncoder` (adds `\n`).
- `Format` must always print exactly 3 fractional digits and `+09:00` (not `Z`), even for UTC inputs. Truncate (not round) to milliseconds.
- `ParseInput` accepts: `2020-05-01T12:34:56.789+09:00`, `2020-05-01T12:34:56+09:00`, `2020-05-01T12:34:56.789Z`, `2020-05-01`. Anything else returns an error (callers map it to `invalid_parameter`).
- `cli.Run`: `version` prints `punchflow <Version()>` + newline to stdout, exit 0. `serve`, `fixtures`, `token` print `not implemented` to stderr and return 1 (PF-16 replaces them). No args or unknown command: usage to stderr, exit 2.
- `main.go`: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, then `os.Exit(cli.Run(...))`. Keep it under 20 lines.
- `embed.go` must use the `all:` prefix so the `.gitkeep` files satisfy the patterns; otherwise `go build` fails on a fresh clone.
- `.gitignore`: append `/web/dist/app/` and `/.punchflow-data/`. Do not remove existing lines.
- `mise.toml`: keep every existing task and the `[tools]` block unchanged except the tasks below. Use `dir = "web"` for web tasks (not `cd`). Add/modify exactly:
  - `web:install` -> `bun install --frozen-lockfile` (dir web)
  - `web:build` (depends `web:install`) -> `bun run build`
  - `web:test` (depends `web:install`) -> `bun test`
  - `web:typecheck` (depends `web:install`) -> `bun run typecheck`
  - `build` (depends `web:build`) -> `go build -o punchflow ./cmd/punchflow`
  - `dev` (depends `web:build`) -> `go run ./cmd/punchflow serve --enable-local-login --enable-admin-reset`
  - `run` -> `mise run build && ./punchflow serve`
  - `test` -> `go test ./...` then `mise run web:test`
  - `lint` -> `golangci-lint run ./...`, `go vet ./...`, `mise run web:typecheck`
  - `keycloak:up` -> `docker compose -f deploy/keycloak/docker-compose.yml up -d`; `keycloak:down` -> `... down`

## Tests (input -> expected)

- `Format(2020-05-01 03:34:56.789123 UTC)` -> `2020-05-01T12:34:56.789+09:00`
- `Format(t with 0 ns)` -> ends with `.000+09:00`
- each of the 4 input formats -> parses; `2020/05/01` -> error
- `FormatDate(2020-05-01T20:00Z)` -> `2020-05-02`
- `kferr.Write(w, kferr.ValidationFailed({"comment":["must not be empty"]}))` -> 422, body `{"code":"validation_failed","message":"comment must not be empty","errors":{"comment":["must not be empty"]}}`
- `kferr.Write(w, errors.New("boom"))` -> 500 `internal_server_error`
- message containing `<a&b>` -> body bytes contain `<a&b>`, no trailing `\n`
- `cli.Run(["version"])` -> 0 and stdout `punchflow 0.1.0\n`; `cli.Run(["nope"])` -> 2
- `embed_test.go`: `fs.Stat(WebDist, "web/dist/.gitkeep")` and `fs.Stat(DefaultFixtures, "fixtures/default/.gitkeep")` succeed

## Verification (each must exit 0; record exit status and log path)

```bash
go build ./... 2>&1 | tee /tmp/pf01-build.log
go vet ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... ./cmd/... . 2>&1 | tee /tmp/pf01-vet.log
go test ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... . 2>&1 | tee /tmp/pf01-test.log
gofmt -l internal/kftime internal/kferr internal/build internal/cli cmd embed.go embed_test.go   # must print nothing
mise tasks ls 2>&1 | tee /tmp/pf01-mise.log   # must list web:install web:build web:test web:typecheck build dev test lint keycloak:up keycloak:down
```

## Done Criteria

- [x] All files in the table exist; `cmd/punchflow/.gitkeep` removed
- [x] Contract symbols exist with the exact names/signatures above
- [x] All listed tests exist and pass; gofmt clean
- [x] `mise tasks ls` shows the new tasks; existing tasks still present

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (ownership, fresh reads + sha256, intent snapshot, drift detection, no git state changes, own progress log only).

## Progress Log

### Session: 2026-10-20 PF-01 implementation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-01/attempt-1/intent.md`; node-boundary snapshot copied from `tmp/riela-fanout/40C5086C-DF26-43B5-86A2-D15F25DEC90E/AC5288BF-5149-4013-AC12-CC12439FCDBF.json`.
**Tasks Completed**: Implemented the stdlib JST clock/time helpers, kickflow error constructors and response writer, embedded version, CLI skeleton, signal-aware entry point, embedded filesystem anchors, mise tasks, and gitignore entries. Added focused tests. `kferr.Write` preserves a non-nil empty validation error map as `"errors":{}`. Removed `cmd/punchflow/.gitkeep`. Preserved concurrent wave-1 changes in other owners' files.
**Verification**: `go build ./...` exit 0 (`/tmp/pf01-build-final.log`); `go vet ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... ./cmd/... .` exit 0 (`/tmp/pf01-vet-final.log`); `go test ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... .` exit 0 (`/tmp/pf01-test-final.log`); `go test -count=1 -v ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... .` exit 0, 13 passing top-level tests (`/tmp/pf01-test-verbose-final.log`); `gofmt -l internal/kftime internal/kferr internal/build internal/cli cmd embed.go embed_test.go` exit 0 with no output (`/tmp/pf01-gofmt-final.log`); `mise tasks ls` exit 0 with all required tasks and existing tasks listed (`/tmp/pf01-mise-final.log`). Post-modify agent `pf01_post_modify_check` independently repeated build, scoped vet, tests, gofmt, and task-list checks successfully; its logs are `/tmp/pf01-postcheck-build.log`, `/tmp/pf01-postcheck-vet.log`, `/tmp/pf01-postcheck-test.log`, and `/tmp/pf01-postcheck-mise.log`.
**Reconciliation requests**: -

### Session: 2026-10-20 PF-01 current-source verification
**Tasks Completed**: Re-verified the assigned PF-01 contract on the shared current source tree; no source edits were needed. Preserved other plans' concurrent changes.
**Verification**: `go build ./...` exit 0 (`/tmp/pf01-build-current.log`); scoped `go vet` exit 0 (`/tmp/pf01-vet-current.log`); uncached verbose scoped tests exit 0, 13/13 top-level tests passed (`/tmp/pf01-test-current.log`); scoped `gofmt -l` exit 0 with no output (`/tmp/pf01-gofmt-current.log`); `mise tasks ls` exit 0 with all PF-01 required tasks present (`/tmp/pf01-mise-current.log`).
**Per-edit intent**: `tmp/punchflow-kickflow-dummy-s460/PF-01/attempt-2/edit-intent.md`; pre-edit plan hash: `tmp/punchflow-kickflow-dummy-s460/PF-01/attempt-2/plan-before-sha256.txt`.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
