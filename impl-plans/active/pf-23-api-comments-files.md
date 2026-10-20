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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf23-build.log
go vet ./internal/api/... ./internal/files/... ./internal/store/... 2>&1 | tee /tmp/pf23-vet.log
go test -race ./internal/files/... 2>&1 | tee /tmp/pf23-files.log
go test -race -run 'Test(Comment|Comments|File|Files|Upload)' ./internal/api/v1/... 2>&1 | tee /tmp/pf23-test.log
gofmt -l internal/api/v1/comments*.go internal/api/v1/files*.go internal/files internal/store/files.go   # prints nothing
```

## Done Criteria

- [ ] 7 operations implemented; download handler works; comment events proven through the receiver; tests pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. Shared paths: `internal/api/v1/comments.go`, `internal/api/v1/files.go`, `internal/files/files.go` (stubs from PF-10).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
