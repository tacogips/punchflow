# PF-23 /v1 Comments and Files Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-14, PF-15, PF-16 | **Plan ID**: PF-23
**Design Reference**: design-docs/specs/design-api-coverage.md rows 71-72, 91-95, event matrix rows 12-13; design-docs/specs/architecture.md#files; notes.md INF-22, INF-26, INF-27, INF-30, INF-37, INF-39
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Implement the comment endpoints (thin handlers over the engine comment methods, so `comment_created`/`comment_updated` are emitted in the same tx) and file upload/metadata, plus the real `internal/files` store and the signed download handler that PF-10 stubbed and PF-16 already mounts.

## Non-goals

- No virus scanning or preview generation, no purge of unreferenced uploads (INF-26). Do not change `internal/server` wiring; `files.New` keeps the PF-10 signature.

## Files

`internal/api/v1/comments.go`, `internal/api/v1/files.go` (shared: replace stubs); tests `comments_test.go`, `files_test.go` (`package v1_test`); `internal/files/files.go` (shared: replace stub), `internal/files/download.go`, `internal/files/files_test.go` (new); `internal/store/files.go` (new: insert/get file by signed id, attachment lookups).

## Path Declarations

### writePaths
- `impl-plans/active/pf-23-api-comments-files.md`
- `internal/api/v1/comments_test.go`
- `internal/api/v1/files_test.go`
- `internal/files/download.go`
- `internal/files/files_test.go`
- `internal/store/files.go`

### sharedPaths
- `internal/api/v1/comments.go`
- `internal/api/v1/files.go`
- `internal/files/files.go`

### sharedPathNotes
- `internal/api/v1/comments.go`: PF-10 stub file; replace the bodies of operations 91-95
- `internal/api/v1/files.go`: PF-10 stub file; replace the bodies of operations 71-72
- `internal/files/files.go`: PF-10 contract stub; implement the store keeping the New/Save/SignedURL/DownloadHandler/MaxBytes signatures, and set `present.AttachmentURL` in `New`

### artifactRoots
- (none)

## Behavior

- `uploadFile`: `auth` any; tenant plan `standard` -> 403 `upgrade_required` (INF-27); `multipart/form-data` required (else 400 `invalid_content_type`); field `file` missing -> 400 `invalid_parameter`; size over `MaxBytes` -> 400 `payload_too_large` (use `http.MaxBytesReader` + `r.ParseMultipartForm`); `Files.Save` writes `<dataDir>/files/<uuid>`, computes base64 MD5 checksum, random `signedId` (32 bytes base64url), content type from the part header or `http.DetectContentType`. Response `{"signedId": ...}`. Audit `file.upload`.
- `getFile`: file must be attached (`file_attachments`) to a ticket the caller can view (`Engine.CanView`), else 404 `resource_not_found` (unattached) or 403 `invisible_ticket`. Response: `url` (`SignedURL`), `filename`, `checksum`, `byteSize`, `contentType`, `createdAt`.
- `SignedURL`: token = base64url(fileID + "." + expiryUnix + "." + hex HMAC-SHA256(key, fileID+"."+expiry)); expiry = now + 5 min. `DownloadHandler`: GET `/files/download/{token}`: bad signature -> 403, expired -> 403, then serve the file with `Content-Disposition: attachment; filename*=UTF-8''<escaped>`.
- `files.New` also sets `present.AttachmentURL = s.SignedURL` (the hook declared by PF-15), so comment and ticket attachments render URLs.
- Comments: `listComments` (CanView; paginated; sortBy createdAt; default createdAt asc), `getComment`, `createComment` (`body` required; `files` up to 10 signed ids -> else 400) -> `Engine.AddComment`; `updateComment` -> `Engine.EditComment`; `deleteComment` -> `Engine.DeleteComment` (empty 200). Enqueue returned events in the tx, Notify after commit. Audit `comment.create|update|delete` with TicketID.

## Key Points and Pitfalls

- Path traversal: store files only under generated UUID names; never use the client filename on disk.
- Compare HMACs with `hmac.Equal`.
- Multipart parsing: limit memory (`ParseMultipartForm(8 << 20)`), remove temp files (`r.MultipartForm.RemoveAll()`).

## Tests

Create tickets in tests with `engine.New(env.Clock).Create` inside `env.DB.WithTx` (actor = sales-rep), not via `POST /v1/tickets`, which PF-22 builds in parallel.

- `internal/files`: Save -> file on disk, checksum = base64 MD5; SignedURL valid within 5 min (fake clock), tampered -> 403, expired (+6 min) -> 403
- upload 1 KiB text -> 200 `signedId`; JSON content type -> 400 `invalid_content_type`; no `file` part -> 400; over limit (env with small MaxUploadBytes option, or a 51 MiB stream) -> 400 `payload_too_large`; tenant plan standard -> 403 `upgrade_required`
- getFile unattached -> 404; attach via the expense-claim `receipt` input when creating the ticket with the engine -> getFile 200, `url` downloads identical bytes
- createComment on expense ticket -> 200 Comment with `mentionedUsers` for `<@accountant-id>`; receiver gets `comment_created` with `data.comment` and `data.ticket`; leave-request ticket -> 422 `errors.base`
- updateComment by non-author -> 403 `missing_permission`; author -> 200 + `comment_updated`; delete -> 200 empty, list shows `deletedAt` and empty body, receiver gets `comment_updated`
- listComments pagination headers; invisible ticket -> 403 `invisible_ticket`

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-14, PF-15, PF-16.
**State at 410d3d8**: not started (PF-10 stubs; `internal/files/files.go` is the 33-line contract stub). Implement this plan as written. Session-462 points:

- Comment events are exactly `comment_created` and `comment_updated`. Deleting a comment emits `comment_updated` with `deletedAt` set. There is no `comment_deleted` event; assert that no received body has that `eventType`.
- Name API tests `TestComment*`, `TestComments*`, `TestFile*`, `TestFiles*`, `TestUpload*`; prefix helpers with `pf23`. `internal/files` tests may use any `TestXxx` names (PF-23 owns that package).

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf23-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... ./internal/files/... 2>&1 | tee /tmp/pf23-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/files/... 2>&1 | tee /tmp/pf23-s462-files.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Comment|Comments|File|Files|Upload)' ./internal/api/v1/... 2>&1 | tee /tmp/pf23-s462-test.log"; echo "exit=$?"
for f in /tmp/pf23-s462-files.log /tmp/pf23-s462-test.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/api/v1/comments.go internal/api/v1/files.go internal/api/v1/comments_test.go internal/api/v1/files_test.go internal/files internal/store/files.go   # prints nothing
rg -n 'not implemented' internal/api/v1/comments.go internal/api/v1/files.go internal/files/files.go   # prints nothing (exit 1)
```

### Done Criteria (session 462)

- [ ] 7 operations implemented; download handler works; `comment_created`/`comment_updated` proven through the receiver (no `comment_deleted`)
- [ ] Both test logs pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-14, PF-15 and PF-16 (all wave A).
**State at f82e374**: not started. `comments.go` and `files.go` in `internal/api/v1` are PF-10 stubs, and `internal/files/files.go` is the PF-10 contract stub. `internal/files/download.go` and `internal/store/files.go` do not exist.
**Path Declarations**: unchanged. 9 declared files (< 300).

1. Implement the whole plan above, with the session-462 points. Keep the `internal/files` contract signatures (`New`, `Save`, `SignedURL`, `DownloadHandler`, `MaxBytes`), because PF-16's server wiring calls them.
2. Comment API tests assert that no received body has `eventType` `comment_deleted`. That literal may appear in your `*_test.go`; it must never appear in production code.
3. Do not edit `internal/engine/*` (PF-14), `internal/present/*` (PF-08/PF-15), `internal/testutil/*` or `internal/server/*` (PF-16), or `internal/api/v1/api.go`, `routes.go` or `routes_test.go` (PF-10). Defects in those become reconciliation requests (README rule 12).
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-23/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0 in both the files and API logs, and the `not implemented` rg prints nothing
- [ ] The progress log has the intent snapshot, pre/post hashes of the three shared files, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-14, PF-15, PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 71-72 and 91-95; `architecture.md#files`; INF-22, INF-26, INF-27, INF-30, INF-37 and INF-39 in `notes.md`.

**Intent**: unchanged. Implement 7 operations (comments, files), the signed download handler, and `comment_created`/`comment_updated` delivery through the receiver. There is no `comment_deleted` event.

**Session-464 changes**:
1. `internal/api/v1/comments.go`, `internal/api/v1/files.go` and `internal/files/files.go` are in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty.
   - Keep the `files.New/Save/SignedURL/DownloadHandler/MaxBytes` signatures, because `internal/server/server.go` (PF-16) calls them.
   - Set `present.AttachmentURL` in `New`.
2. Never edit `internal/engine/*`, `internal/present/*`, `internal/webhook/*`, `internal/testutil/*`, `internal/server/*`, `api.go`, `routes.go` or `routes_test.go`. Missing behavior there is a reconciliation request to its owner.
3. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet <pkg>`.
   - Log each result.
   - Prefixes: unexported identifiers start with `pf23`. New exported store functions start with `File`.
4. Guards:
   - The whole `./internal/files/...` package must pass.
   - The manifest `-run 'TestPF10'` line must PASS.
   - Logs are `/tmp/pf23-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-23/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0 in both the files and API logs; PF10 guard PASS; the `not implemented` rg empty
- [ ] Pre/post hashes of the three transferred files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B2 (with PF-22, PF-24). **Depends on**: PF-14a, PF-14b, PF-14c, PF-15, PF-16. The single PF-14 dependency is replaced by its three split plans. Your tests create tickets through `Engine.Create`, which builds the route (PF-14b) and validates the `receipt` file reference (PF-14c). The comment methods are PF-14a's.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (9 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: the 7 comment/file operations, the download handler, and `comment_created`/`comment_updated` proven through the receiver (no `comment_deleted`). Logs are `/tmp/pf23-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-23/attempt-<n>/`.

**Pitfalls added in s465**:
- Since PF-14c, `Engine.Create` rejects unknown file signed ids (`422 validation_failed`, `errors.receipt`). Upload or insert the file first (`POST /v1/files` in an httptest test, or your own `internal/store/files.go` helper), then reference its `signedId`.
- Never edit `internal/engine/*`. An engine defect is a reconciliation request to the owner by file, as listed in PF-22's s465 section.

### Done Criteria (session 465)

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard, files and API tests exit 0, FAIL=0, SKIP=0, PASS>0; the `not implemented` check exit 0; gofmt check exit 0
- [x] Pre/post hashes of the three transferred files; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). `internal/api/v1/comments.go`, `internal/api/v1/files.go` and `internal/files/files.go` were transferred from PF-10 in session 464.

## Progress Log

### Session: 2026-10-21 (s465, PF-23 implementation)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-23/attempt-1/intent-snapshot.md` (per-edit intentions 1-20; dispatch snapshot: `tmp/riela-fanout/49444EA1-727B-466F-BA6C-A2DD10EB6248/2DD38E7E-7FC6-4B9E-9124-13D294CF4902.json`)
**Tasks Completed**:
- Implemented the 7 comment/file operations, comment-created/updated receiver delivery, signed downloads, attached-file visibility and multipart upload.
- Added store, API httptest and file-package tests, including `comment_deleted` absence, file-reference attachment, signature tampering and expiry.
- All changes remain within PF-23's declared paths; no reconciliation requests.
**Verification** (final-source logs under `/tmp`; complete copies under `tmp/punchflow-kickflow-dummy-s465/PF-23/attempt-1/`):
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf23-s465-build-final.log'; echo "exit=$?"` — exit 0.
- `bash -o pipefail -c 'go vet ./internal/api/v1/... ./internal/files/... 2>&1 | tee /tmp/pf23-s465-vet-final.log'; echo "exit=$?"` — exit 0.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...` — exit 0, PASS=2 FAIL=0 SKIP=0; `/tmp/pf23-s465-pf10guard-final.log`.
- `go test -count=1 -v -race ./internal/files/...` — exit 0, PASS=2 FAIL=0 SKIP=0; `/tmp/pf23-s465-files-final.log`.
- `go test -count=1 -v -race -run 'Test(Comment|Comments|File|Files|Upload)' ./internal/api/v1/...` — exit 0, PASS=5 FAIL=0 SKIP=0; `/tmp/pf23-s465-test-final.log`.
- `test -z "$(gofmt -l internal/api/v1/comments.go internal/api/v1/files.go internal/api/v1/comments_test.go internal/api/v1/files_test.go internal/files/files.go internal/files/download.go internal/files/files_test.go internal/store/files.go)"` — exit 0.
- The must-pass `not implemented` check exits 0 (rg no-match handled); all 8 Go files are below 1000 lines. Full logs: `stub-check-final.log`, `gofmt-final.log`, `line-counts.txt`.
- Every production write had a logged `go build ./...` gate; every test write had its package `go vet` gate. Final source hashes: `final-source-hashes.txt`; transferred-file pre-hashes: `pre-existing-hashes.txt`.
- Earlier test-authoring failures were corrected: uploader FK setup, a single-connection transaction deadlock, original-token expiry assertion, and test fixtures/permissions/content type. Those attempts are retained as prior logs; the final-source suites above pass uncached.
**Reconciliation requests**: None.
**Implementation status**: Complete for PF-23. Formal review, serial integration review, and PF-27 final gate are downstream workflow steps.
