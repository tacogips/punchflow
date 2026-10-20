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

- [ ] Signatures unchanged; all listed OIDC tests pass uncached with `-race`; PF-05 tests still pass; counts recorded; no go.mod/go.sum change

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. `internal/auth/oidc.go` is shared with PF-05 (stub creator): record its hash before editing.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
