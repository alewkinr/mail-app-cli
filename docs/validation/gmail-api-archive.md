# Gmail API archive validation

Date: 2026-07-31

Branch: `plan/gmail-api-archive`

Automated validation owner: Codex

Overall status: **Automated checks complete; manual checks pending**

TASK-045 through TASK-047 require user-controlled Gmail test accounts and
consent. The pre-existing CON-006 AppleScript compile failure was repaired and
the complete repository test suite now passes.

## 2026-07-31 token-exchange correction

A secret-free diagnostic POST to Google's token endpoint used the configured
Desktop client ID plus deliberately invalid code/verifier values. Google
returned `invalid_request` with `client_secret is missing`, proving that this
specific Desktop client requires its Google-issued client secret for token
exchange. No user authorization code, token, Gmail credential, or client-secret
value was sent or recorded by that diagnostic.

The corrected build injects both Desktop client credentials. Unit tests prove
that token exchange and refresh include the client secret only in their POST
forms, while authorization URLs, Keychain records, status, and errors do not.

Profile validation now preserves the Gmail API's safe HTTP status metadata.
HTTP 403 reports explain that the maintainer must enable the Gmail API for the
OAuth client's project, then check Workspace API policy and project quota if
the API is already enabled. Raw provider response bodies remain redacted.

## Automated checks

Commands that use `GOCACHE=/private/tmp/...` use an isolated writable build
cache only; it does not change the command's packages or build configuration.

| Command | Exit | Result |
| --- | ---: | --- |
| `gofmt -w <every changed Go file>` | 0 | All changed Go sources formatted. |
| `go vet ./...` | 0 | Passed with no findings. |
| `go test ./internal/gmailapi ./internal/gmailauth ./internal/archive ./cmd` | 0 | All four targeted packages passed. Tests used only loopback fake servers and fake dependencies. |
| `go test -race ./internal/gmailauth ./internal/archive ./cmd` | 0 | OAuth callback, refresh persistence, routing, and command tests passed under the race detector. |
| `CGO_ENABLED=0 go test ./internal/gmailauth` | 0 | Unsupported-platform Keychain path and OAuth tests passed. |
| `GOOS=linux CGO_ENABLED=0 go test -run '^$' -exec=true ./...` | 0 | Every package compiled for the non-Darwin, non-CGO path. |
| `go test ./...` with a fresh isolated build cache | 0 | All 229 tests passed across seven packages after the CON-006 syntax repair. |
| `make build-gmail` with blank `GMAIL_OAUTH_CLIENT_ID` | 2 | Expected failure: `GMAIL_OAUTH_CLIENT_ID is required`. |
| `make build-gmail` with blank `GMAIL_OAUTH_CLIENT_SECRET` | 2 | Expected failure: `GMAIL_OAUTH_CLIENT_SECRET is required`. |
| `make build-gmail` with generated fake Desktop client ID and secret | 0 | Gmail-enabled binary built with both link-time application credentials; the build command did not echo linker values. |
| `git diff --check` | 0 | No whitespace errors. |
| `rg -l 'GOCSPX-[A-Za-z0-9_-]+' .` | 1 | Expected no-match exit; no Google Desktop client-secret value is present in repository files. |
| `git grep -nE 'ya29\\.\|1//[A-Za-z0-9_-]{20,}\|AIza[0-9A-Za-z_-]{20,}' -- .` | 1 | Expected no-match exit; tracked files contain no token-like values. |
| `rg -n 'ya29\\.\|1//[A-Za-z0-9_-]{20,}\|AIza[0-9A-Za-z_-]{20,}' .` | 1 | Additional no-match check including untracked implementation files. |

## CON-006: resolved AppleScript failure

Command:

```bash
go test ./...
```

Original failure:

```text
--- FAIL: TestEmbeddedScripts (0.69s)
    --- FAIL: TestEmbeddedScripts/get_accounts (0.07s)
        scripts_test.go:64: failed to compile embedded script: exit status 1
            compilation error: Expected expression, “)”, etc. but found “try”. (-2741)
FAIL
FAIL    github.com/intelligrit/mail-app-cli/pkg/mail
```

Resolved on 2026-07-31 by moving both statement-form `try` blocks before the
AppleScript record literal and assigning their results to variables. The
script's output fields are unchanged, `TestEmbeddedScripts` was not weakened,
and all 24 embedded-script compilation cases pass.

## Automated security assertions

- OAuth authorization requests contain exactly one `gmail.modify` scope and
  PKCE-S256 and contain no Desktop client secret.
- Login prints the authorization URL to stdout and has no browser-opening
  dependency or production launcher.
- The loopback callback page reports only that the response was received and
  directs the user back to the terminal; it does not claim that profile
  validation or Keychain persistence succeeded.
- Token exchange and refresh form tests use exact key allowlists, include both
  Desktop client credentials as form parameters, and use no HTTP Basic
  authorization.
- Provider failures retain only validated OAuth error codes; descriptions,
  bodies, authorization codes, and client-secret values remain redacted.
- Login owns exactly one post-validation Keychain write; the authorizer owns
  none.
- The Desktop client secret is supplied only through the maintainer build,
  is not stored in per-account Keychain records, and has no runtime flag or
  credential-file input.
- Authorization records are keyed by stable Mail.app account ID in native
  non-synchronizing, while-unlocked Keychain items.
- No authorization, refresh, revocation, status, or Gmail API error emits
  generated token or provider-response fixtures.
- Zero, multiple, paginated, blank-ID, malformed, unauthorized, rate-limited,
  and oversized Gmail lookups invoke no mutation.
- Every linked-account failure invokes the Mail.app archiver zero times.
- Only a Keychain `ErrNotFound` selects the Mail.app fallback, which receives
  the original local message ID.
- macOS Keychain parameter, unavailable, and authentication failures preserve
  their typed causes and provide login-Keychain lock/unlock recovery guidance.

## Manual validation status

These checks were not run because they require a maintainer-provisioned OAuth
client plus user-controlled dedicated Gmail accounts and explicit consent.
No credentials were requested, synthesized, or read.

### TASK-045: end-to-end dedicated account

Status: **Pending**

- Login using only `--account`.
- Confirm no credential-file or application-secret input exists.
- Inspect only redacted status or Keychain metadata.
- Archive one inbox message by local Mail.app ID.
- Confirm only `INBOX` is removed in Gmail and after Mail.app sync.
- Confirm every other label remains.
- Confirm a second archive returns not-found without another mutation.

### TASK-046: two-account isolation

Status: **Pending**

- Link dedicated Gmail accounts A and B to their distinct Mail.app account IDs.
- Archive one B message.
- Confirm B's authorization/backend is used, A is untouched, and only B's
  selected message loses `INBOX`.

### TASK-047: propagated revocation

Status: **Pending**

- Revoke the grant through Google Account settings while retaining Keychain.
- Poll redacted status every 30 seconds for at most 10 minutes.
- After reauthorization-required appears, confirm archive returns exactly one
  account-specific login command and never falls back to Mail.app.
- If propagation is not observed in the window, record the result as
  inconclusive.
- Reauthorize and confirm archive succeeds.

The manual procedure must not invoke `/usr/bin/security`.
