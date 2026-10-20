# PF-17 Keycloak OIDC Login Implementation Plan

**Status**: Ready
**Wave**: 3 | **Depends On**: PF-05 | **Plan ID**: PF-17
**Design Reference**: design-docs/specs/design-auth-admin.md#keycloak-oidc-authorization-code--pkce, #browser-sessions; design-docs/specs/notes.md INF-31
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Implement browser SSO against Keycloak using authorization code + PKCE (S256) as a backend-for-frontend: the server keeps state, nonce and the code verifier, exchanges the code, verifies the ID token, maps the e-mail to a punchflow user and creates a session. PF-05 created `internal/auth/oidc.go` as a stub with fixed signatures and already routes `/auth/oidc/login` and `/auth/oidc/callback` to it. Use `github.com/coreos/go-oidc/v3/oidc` and `golang.org/x/oauth2` (already in go.mod from PF-02).

## Non-goals

- No JIT user provisioning, no RP-initiated Keycloak logout, no refresh tokens, no Keycloak container in automated tests.
- Do not edit other `internal/auth` files (log a reconciliation request if a PF-05 store helper is missing).

## Files

`internal/auth/oidc.go` (shared: replace the stub bodies, keep `OIDCConfig`, `NewOIDC`, `Login`, `Callback` signatures), `internal/auth/oidc_test.go` (new), `internal/auth/oidc_fake_test.go` (new: fake issuer helper).

## Path Declarations

### writePaths
- `impl-plans/active/pf-17-auth-oidc.md`
- `internal/auth/oidc_test.go`
- `internal/auth/oidc_fake_test.go`

### sharedPaths
- `internal/auth/oidc.go`

### sharedPathNotes
- `internal/auth/oidc.go`: created by PF-05 as a stub; replace the bodies, keeping `OIDCConfig`, `NewOIDC`, `Login` and `Callback` signatures

### artifactRoots
- (none)

## Behavior

- `NewOIDC`: empty issuer -> `(nil, nil)`. Otherwise `oidc.NewProvider(ctx, issuer)` (discovery), `oauth2.Config{ClientID, ClientSecret, RedirectURL, Endpoint: provider.Endpoint(), Scopes: openid email profile}`, verifier `provider.Verifier(&oidc.Config{ClientID})`. Discovery failure -> error (serve exits 1 with the message).
- `Login`: `returnTo` must start with `/` and not `//` (else `/`). Generate state (32 random bytes, base64url), nonce, and verifier (`oauth2.GenerateVerifier()`). Insert an `oidc_states` row expiring in 10 min (`store.InsertOIDCState`). Redirect 302 to `AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))`.
- `Callback`: `error` query param from the IdP -> 400 HTML page. `store.TakeOIDCState(state)` (single use) missing/expired -> 400. `Exchange(ctx, code, oauth2.VerifierOption(verifier))`. Get the `id_token` extra; `verifier.Verify`; nonce must match. Claims `email` required (missing -> 403). Find the user by e-mail (case-insensitive) -> none -> 403 page "no punchflow user for <email>". `ActivateOnLogin` (invited+valid -> activated; inactive -> 403). `CreateSession`, set the cookie exactly as local login does, 302 to `returnTo`.
- Error pages are minimal HTML (`text/html; charset=utf-8`) without echoing raw tokens.

## Key Points and Pitfalls

- Do not hand-roll JWT verification; rely on go-oidc (`iss`, `aud`, `exp`, signature via JWKS).
- The PKCE verifier must never leave the server; only the S256 challenge goes into the redirect.
- Use the authenticator's clock for state expiry. go-oidc checks `exp` with its own clock; in the expired-token test, mint a token whose `exp` is in the real past.
- Use `db.Q()` (no tx is open in these handlers).

## Fake issuer (test helper)

An `httptest.Server` serving `/.well-known/openid-configuration` (issuer = server URL, `authorization_endpoint`, `token_endpoint`, `jwks_uri`), `/jwks` (RSA 2048 key generated in the test), and `/token`, which validates `code` and checks that `base64url(sha256(code_verifier))` equals the challenge recorded when the test followed the `/authorize` redirect. It then returns `{"access_token","token_type":"Bearer","id_token"}` signed with RS256 (build the JWT with `crypto/rsa` + `encoding/base64` in the test; no extra dependency). Configurable claims: email, nonce, aud, exp.

## Tests (input -> expected)

- success: Login -> 302 with `code_challenge_method=S256`, no `code_verifier` in the URL; Callback -> 302 to returnTo, Set-Cookie `punchflow_session`; `GET /auth/session` with the cookie -> 200
- unknown state -> 400; expired state (fake clock +11 min) -> 400; replayed state -> 400 on the second use
- nonce mismatch -> 403; wrong audience -> 403; expired ID token -> 403
- unknown e-mail -> 403; suspended user -> 403; invited user with valid invitation -> activated and logged in
- `returnTo=//evil.example` -> redirect target `/`
- empty issuer -> `NewOIDC` returns nil and the routes answer 404 `endpoint_not_found`

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 3. **Depends on**: PF-05.
**State at 410d3d8**: not started. `internal/auth/oidc.go` is the 30-line PF-05 stub. Implement this plan as written. `ActivateOnLogin` (PF-05) writes `model.UserStatusActive` (`activated`); the invited-user test asserts `activated`. Name the new tests `TestOIDC*`. Do not edit other PF-05 files.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf17-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-test.log'; echo "exit=$?"   # whole package: oidc.go is shared with PF-05, whose tests must stay green
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf17-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf17-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf17-s462-test.log)"
grep -c -- '--- PASS: TestOIDC' /tmp/pf17-s462-test.log   # >= 6 (success, state cases, token cases, user cases, returnTo, empty issuer)
gofmt -l internal/auth   # prints nothing
git diff --quiet -- go.mod go.sum && echo "go.mod unchanged"
```

### Done Criteria (session 462)

- [x] Signatures unchanged; all listed OIDC tests pass uncached with `-race`; PF-05 tests still pass; counts recorded; no go.mod/go.sum change

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. `internal/auth/oidc.go` is shared with PF-05 (stub creator): record its hash before editing.

## Progress Log

### Session: 2026-10-20 PF-17 implementation (session 462)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s462/PF-17/attempt-1/edit-intent-001.md` through `edit-intent-009.md`; pre-edit hashes in `tmp/punchflow-kickflow-dummy-s462/PF-17/attempt-1/pre-edit-sha256.txt`.
**Tasks Completed**: Replaced the OIDC stub while preserving the public signatures. Added discovery and provider verification; random single-use state/nonce and server-only PKCE verifier; ten-minute state expiry; same-origin returnTo validation; callback code exchange and ID-token signature, issuer, audience, expiry and nonce verification; case-insensitive email lookup with unknown users rejected and operational lookup errors returned as generic 500 pages; invitation activation, inactive-user rejection, session creation and the local-login-equivalent cookie. Added fake issuer and behavioral coverage for PKCE success, session cookie and session read, unknown/expired/replayed state, nonce/audience/expired-token failures, unknown and suspended users, valid invited-user activation, open-redirect fallback, and disabled issuer routes.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf17-s462-build.log'` -> exit=0; complete log `/tmp/pf17-s462-build.log` (empty).
- `bash -o pipefail -c 'go vet ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-vet.log'` -> exit=0; complete log `/tmp/pf17-s462-vet.log` (empty).
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-test.log'` -> exit=0; complete log `/tmp/pf17-s462-test.log`; PASS=33, FAIL=0, SKIP=0, cached lines=0; `TestOIDC*` PASS=11.
- `gofmt -l internal/auth` -> exit=0, no files listed.
- `git diff --quiet -- go.mod go.sum && echo "go.mod unchanged"` -> exit=0, module files unchanged.
**Reconciliation requests**: None.

### Session: 2026-10-20 PF-17 repair attempt 1 (comm-008208)
**Finding addressed**: PF17-TI-01. In `internal/auth/oidc_test.go`, the suspended-user token now uses the fixture email `sus@example.test`; case-specific `wantBody` assertions distinguish unknown-email (`no punchflow user for missing@example.test`) from inactive-user rejection (`user is not eligible to log in`). Both 403 subtests remain present. Edit intent and pre-edit hashes: `tmp/punchflow-kickflow-dummy-s462/PF-17/attempt-2/edit-intent-001.md`, `pre-edit-sha256.txt`; plan append intent: `edit-intent-002.md`.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf17-s462-repair-build.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-repair-build.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-repair-build.log`.
- `bash -o pipefail -c 'go vet ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-repair-vet.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-repair-vet.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-repair-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-repair-test.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-repair-test.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-repair-test.log`; PASS=33, FAIL=0, SKIP=0, cached lines=0; `TestOIDC*` PASS=11.
- `gofmt -l internal/auth` -> exit=0, no files listed. `git diff --quiet -- go.mod go.sum` -> exit=0, unchanged.
- `internal/auth/oidc.go` remains sha256 `2b415a90bddca0057f3f13aced3cda94c95f88be1fe55cfe0ac831b8850f86a8`; PF-05 files `middleware_test.go`, `session_test.go`, `token_test.go` hashes remain `5db49f8c9477f20f69973201421c9a679154e8962e70621b90aacd5af73705cf`, `05df64fd5fb6b1e9e4d9ff6c7124ab84047b1936109baa37d96067d4fbb4f575`, `0f0b51dcd78463197a09f9734628214fb3e95aaa99cd5046f4ccedb30a6ad676`. `go.mod` and `go.sum` hashes are `82d4cfcda01d25848cfea7b1784dd13d5d090ca5f251d28cbcda36970c2a1e8c` and `759e42a16c10aa37c6c542ea9f9fd29fbe4b49820f418633c31e5cf7c6bcd0df`.
**Review status**: PF17-TI-01 is corrected and verified on the current source. Independent integrity/adversarial re-review remains downstream; no review acceptance is claimed.

### Session: 2026-10-20 PF-17 repair attempt 2 (comm-008212)
**Finding addressed**: PF17-ADV-01. `internal/auth/oidc.go` now uses private `oidcSafeReturnTo` to reject protocol-relative URLs, backslashes, C0/DEL controls, parse errors, and parsed scheme/host values while preserving safe relative paths and query strings. `TestOIDCOpenRedirectTargetFallsBackToRoot` now has six subtests; each checks the persisted `return_to` and the callback Location for protocol-relative, backslash-host, backslash-separator, tab, absolute URL and `/dashboard?x=1` inputs. Public signatures and callback behavior are unchanged. Attempt edit intents 001-004 and pre-edit hashes are under `tmp/punchflow-kickflow-dummy-s462/PF-17/attempt-3/`.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf17-s462-adv1-build.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-adv1-build.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-adv1-build.log`.
- `bash -o pipefail -c 'go vet ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-adv1-vet.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-adv1-vet.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-adv1-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/auth/... 2>&1 | tee /tmp/pf17-s462-adv1-test.log; status=$?; printf "EXIT_STATUS=%s\\n" "$status" | tee -a /tmp/pf17-s462-adv1-test.log; exit "$status"'` -> exit=0; complete log `/tmp/pf17-s462-adv1-test.log`; PASS=39, FAIL=0, SKIP=0, cached=0; `TestOIDC*` PASS=17.
- `gofmt -l internal/auth` -> exit=0, no files listed. `git diff --check -- internal/auth/oidc.go internal/auth/oidc_test.go` -> exit=0. `git diff --quiet -- go.mod go.sum` -> exit=0, unchanged.
- PF-05 test hashes remain unchanged: `middleware_test.go` `5db49f8c9477f20f69973201421c9a679154e8962e70621b90aacd5af73705cf`, `session_test.go` `05df64fd5fb6b1e9e4d9ff6c7124ab84047b1936109baa37d96067d4fbb4f575`, `token_test.go` `0f0b51dcd78463197a09f9734628214fb3e95aaa99cd5046f4ccedb30a6ad676`; `go.mod` `82d4cfcda01d25848cfea7b1784dd13d5d090ca5f251d28cbcda36970c2a1e8c` and `go.sum` `759e42a16c10aa37c6c542ea9f9fd29fbe4b49820f418633c31e5cf7c6bcd0df` unchanged.
**Review status**: The repair is implemented and verified on this source. Independent test-integrity and adversarial re-review remain downstream; no review acceptance is claimed.
