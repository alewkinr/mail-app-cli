---
goal: Implement Gmail API archiving with OAuth 2.0 and macOS Keychain-backed token storage
version: 1.2
date_created: 2026-07-26
last_updated: 2026-07-26
owner: Ramil
status: 'In Progress'
tags:
  - feature
  - gmail
  - oauth
  - keychain
  - archive
  - macos
---

> **For agentic workers:** REQUIRED: Use `execute-implementation-plan` to implement this plan.

# Introduction

![Status: In Progress](https://img.shields.io/badge/status-In%20Progress-yellow)

> **Implementation progress (2026-07-26):** TASK-001 through TASK-044 are
> implemented and their automated checks are recorded in
> `docs/validation/gmail-api-archive.md`. TASK-045 through TASK-047 remain
> pending because they require user-controlled Gmail accounts and consent.
> CON-006 was resolved on 2026-07-31 and the complete repository test suite
> passes.

Implement a Gmail-specific archive backend that removes the `INBOX` system label through the Gmail REST API while preserving the existing Mail.app Archive action for accounts that are not explicitly linked to Gmail OAuth.

The maintainer provisions one Desktop OAuth client for `mail-app-cli` and injects its public client ID into release binaries. The user authorizes each Mail.app account once with `mail-app-cli gmail auth login --account "<Mail.app account>"` through Google's browser-based OAuth 2.0 flow. The CLI prints the authorization URL instead of launching a browser. Users do not create Google Cloud projects, download credential files, or provide OAuth client secrets. The CLI never receives a Gmail password or app password. It stores the account mapping, public OAuth client ID, and user token set in a native macOS Keychain generic-password item, refreshes access tokens without user interaction, and requests reauthorization only when Google revokes or expires the grant.

The implementation must not attempt to reuse credentials from macOS Internet Accounts or Mail.app. Apple's Accounts framework is deprecated, Apple directs new integrations to provider APIs, and macOS does not expose Mail.app's Google bearer token to an unrelated CLI through a supported API. An existing Google browser session may reduce the first authorization to account selection and consent, but it does not eliminate the OAuth grant.

## 1. Requirements & Constraints

- **REQ-001**: Preserve `mail-app-cli messages archive <local-message-id> -a <account> -m <mailbox>` as the public archive command.
- **REQ-002**: Treat the archive command's positional message ID only as Mail.app's local numeric message ID. Do not add direct RFC `Message-ID` input support.
- **REQ-003**: For a Mail.app account explicitly linked by `mail-app-cli gmail auth login --account <Mail.app account>`, archive through the Gmail API by removing the `INBOX` label from exactly one Gmail message.
- **REQ-004**: For an account without a Gmail OAuth link, preserve the existing Mail.app UI archive implementation in `pkg/mail/client.go:409-429` and `pkg/mail/scripts/archive.scpt:37-95`.
- **REQ-005**: Add `mail-app-cli gmail auth login`, `mail-app-cli gmail auth status`, and `mail-app-cli gmail auth logout`. Require `--account <Mail.app account>` on `login` and `logout`; support `-a` as the short alias. Keep `--account` optional on `status` so omitting it reports every linked account. Do not expose a client-secret or credential-file flag.
- **REQ-006**: Support multiple Mail.app Gmail or Google Workspace accounts by keying each authorization record to Mail.app's stable account ID returned by `pkg/mail/scripts/get_accounts_json.scpt`.
- **REQ-007**: Resolve a local Mail.app message ID to its RFC `Message-ID` before querying Gmail.
- **REQ-008**: Resolve the Gmail message with `users.messages.list`, `labelIds=INBOX`, `q=rfc822msgid:<value>`, and `maxResults=2`; modify nothing when the query returns zero or multiple results.
- **REQ-009**: Archive the unique Gmail message with `users.messages.modify` and JSON body `{"removeLabelIds":["INBOX"]}`.
- **REQ-010**: Treat an archive as successful only when the modify response does not contain `INBOX` in `labelIds`.
- **REQ-011**: Preserve the successful stdout text `Message archived` so existing wrappers do not need an output migration.
- **REQ-012**: Invalidate the source, `Archive`, and `All Mail` message caches after either backend succeeds, preserving `cmd/messages.go:244-248`.
- **AUTH-001**: Use an OAuth 2.0 Desktop client, Authorization Code flow, a loopback redirect bound to `127.0.0.1` on an ephemeral port, PKCE-S256, and a cryptographically random `state`.
- **AUTH-002**: Request exactly `https://www.googleapis.com/auth/gmail.modify`; do not request `https://mail.google.com/`.
- **AUTH-003**: Request offline access and explicit account selection/consent so the initial login returns a refresh token.
- **AUTH-004**: Print the authorization URL to stdout on its own line, never launch a browser, and keep the callback listener active while the user opens the URL in a browser of their choice.
- **AUTH-005**: Set the OAuth callback timeout to two minutes and shut down the loopback server after one valid callback, one provider error, or timeout.
- **AUTH-006**: Call `users.getProfile` after token exchange and record the authenticated Gmail primary address.
- **AUTH-007**: Require the Gmail profile address to equal one of the selected Mail.app account's email addresses, case-insensitively. Reject a mismatch and provide no override flag.
- **AUTH-008**: On `invalid_grant`, missing refresh token, or revoked access, fail with an actionable message that names `mail-app-cli gmail auth login --account <account>`; never fall back to Mail.app after a linked Gmail account's API authorization fails.
- **SEC-001**: Never accept, request, store, or log a Gmail password or Google app password.
- **SEC-002**: Store the complete user authorization record only in macOS Keychain. The record includes schema version, Mail.app account identity, Gmail profile address, public OAuth client ID, access token, refresh token, token type, and expiry. Do not store the Desktop client secret in the authorization record; inject it only into maintainer-built binaries.
- **SEC-003**: Use native Security.framework bindings through `github.com/keybase/go-keychain`; do not invoke `/usr/bin/security`, because command-based access weakens per-application Keychain access control and may expose secret material through process arguments.
- **SEC-004**: Use Keychain service `com.alewkinr.mail-app-cli.gmail-oauth` and Mail.app account ID as the Keychain account key.
- **SEC-005**: Mark Keychain items accessible only while the user session is unlocked. Do not enable synchronization to iCloud Keychain.
- **SEC-006**: Keep access and refresh tokens out of command arguments, environment variables, stdout, stderr, cache files, test fixtures, and Go error strings. Permit the Desktop client ID and client secret only in the maintainer release build environment. Keep the client secret out of authorization URLs, runtime flags, credential files, Keychain records, logs, status, and errors.
- **SEC-007**: Make status and error output redact OAuth codes, access tokens, refresh tokens, and raw provider responses.
- **SEC-008**: Fail closed when Keychain access returns an error other than item-not-found. Do not silently route to Mail.app. For macOS unavailable/authentication failures, preserve the typed cause and provide lock/unlock recovery guidance for the login Keychain.
- **SEC-009**: Use constant-time equality only where secret comparison is required; email-address comparison is case-insensitive string comparison and is not a secret comparison.
- **SEC-010**: Generate PKCE verifier and OAuth state with `crypto/rand`; reject missing, duplicated, or mismatched state.
- **CON-001**: The maintainer must provision one Google Cloud project, enable Gmail API, configure the OAuth consent screen, and create a Desktop OAuth client. End users must not perform these steps.
- **CON-002**: Release builds require the maintainer-owned Desktop OAuth client ID and Google-issued Desktop client secret through build environment variables `GMAIL_OAUTH_CLIENT_ID` and `GMAIL_OAUTH_CLIENT_SECRET`. Inject them with Go linker variables `github.com/intelligrit/mail-app-cli/internal/gmailauth.appOAuthClientID` and `github.com/intelligrit/mail-app-cli/internal/gmailauth.appOAuthClientSecret`; do not use a runtime credential file. Installed applications cannot keep the Desktop client secret confidential, so it is observable application metadata rather than a user or server-side secret.
- **CON-003**: `gmail.modify` is a restricted Gmail scope. Public distribution requires Google OAuth verification; an External project in Testing issues refresh tokens that expire after seven days.
- **CON-004**: The feature is supported on macOS only. Non-Darwin builds must compile and return a deterministic unsupported-platform error for Keychain-backed Gmail authorization.
- **CON-005**: Native Keychain bindings require CGO for macOS builds. Release and local build documentation must state `CGO_ENABLED=1`.
- **CON-006**: Resolved on 2026-07-31. The worktree baseline had a pre-existing failure in `pkg/mail/TestEmbeddedScripts/get_accounts`: `osacompile` reported `Expected expression, “)”, etc. but found “try”. (-2741)`. The repair assigns guarded values to variables before building the AppleScript record, preserves the output fields, and leaves the compilation test intact.
- **CON-007**: Do not read Mail.app databases, Internet Accounts databases, or unrelated Keychain items to obtain Google credentials.
- **CON-008**: Do not change list, show, delete, move, mark, flag, send, attachment, search, or sync behavior.
- **GUD-001**: Keep Mail.app automation in `pkg/mail`, Gmail HTTP operations in `internal/gmailapi`, OAuth and Keychain operations in `internal/gmailauth`, and backend selection in `internal/archive`.
- **GUD-002**: Pass `context.Context` from Cobra through the archive service, OAuth exchange, token refresh, and Gmail HTTP calls.
- **GUD-003**: Define interfaces at consumer boundaries so OAuth, Keychain, Mail.app, authorization-URL output, and Gmail HTTP behavior are unit-testable without real accounts.
- **GUD-004**: Wrap errors with operation context while preserving typed sentinel errors for not-authorized, not-found, ambiguous-match, unsupported-platform, and reauthorization-required cases.
- **PAT-001**: Continue passing all JXA values through `osascript` argv, following `pkg/mail/client.go:21-49`; do not interpolate user-controlled values into script source.
- **PAT-002**: Embed every new automation source with `//go:embed` and add it to `TestEmbeddedScripts`, following `pkg/mail/embed.go` and `pkg/mail/scripts_test.go:12-66`.

## 2. Implementation Steps

### Implementation Phase 1

- GOAL-001: Add deterministic Mail.app identity resolution and native Keychain persistence primitives without changing archive routing.

| Task | Description | Completed | Date |
| --- | --- | --- | --- |
| TASK-001 | Create `pkg/mail/errors.go` with `ErrAccountNotFound`, `ErrAccountAmbiguous`, `ErrMailboxNotFound`, `ErrMessageNotFound`, and `ErrMessageIDMissing`. Add an unexported `resolverError(code string) error` that maps the exact resolver codes to wrapped sentinels compatible with `errors.Is` and returns a secret-safe unknown-code error. | [ ] | |
| TASK-002 | Create `pkg/mail/scripts/resolve_message_identity.scpt`. Filter `mail.accounts()` for exact-name matches and return `{"error":"account_not_found"}` for zero or `{"error":"account_ambiguous"}` for more than one. Recursively resolve the mailbox by exact name, locate the message only when `String(message.id())` equals the supplied local message ID, and return the same `error` property with `mailbox_not_found`, `message_not_found`, or `message_id_missing` for those exact failures. Return one JSON object containing either identity fields or one error code. Read every account email from `account.emailAddresses()` and pass inputs only through `run(argv)`. | [ ] | |
| TASK-003 | Update `pkg/mail/embed.go` to embed `resolve_message_identity.scpt` as `resolveMessageIdentityJXAScript`. Update `pkg/mail/scripts_test.go` so `TestEmbeddedScripts` compiles the new JXA source. | [ ] | |
| TASK-004 | Create `pkg/mail/message_identity.go` with `MessageIdentity{LocalID string, RFCMessageID string, AccountID string, AccountName string, AccountEmailAddresses []string, MailboxName string}`, exact runner type `type jxaRunFunc func(script string, args ...string) (string, error)`, and `(*Client).ResolveMessageIdentity(accountName, mailboxName, localMessageID string) (*MessageIdentity, error)`. Implement the method through unexported `resolveMessageIdentity(run jxaRunFunc, accountName, mailboxName, localMessageID string) (*MessageIdentity, error)` so tests inject a runner and assert the embedded script plus argv without launching Mail.app. Decode the script's `error` property through `resolverError`. Reject blank account ID, local ID, resolved RFC `Message-ID`, account name, or mailbox name before returning an identity; preserve an empty account-email array so unlinked non-Gmail accounts can still use the Mail.app fallback. | [ ] | |
| TASK-005 | Create `pkg/mail/account_lookup.go` with pure helper `selectAccountByName(accounts []Account, name string) (*Account, error)` and `(*Client).GetAccountByName(name string) (*Account, error)`, which passes `GetAccountsJSON()` results to that helper. Require one exact-name match and wrap `ErrAccountNotFound` or `ErrAccountAmbiguous`. Extend `Account` in `pkg/mail/client.go` with `EmailAddresses []string` while retaining `EmailAddress` for JSON compatibility. | [ ] | |
| TASK-006 | Update `pkg/mail/scripts/get_accounts_json.scpt` to return both the existing first `emailAddress` and the complete `emailAddresses` array. Add `pkg/mail/account_lookup_test.go` proving zero/one/multiple exact-name selection and backward-compatible decoding of `emailAddress`. | [ ] | |
| TASK-007 | Create `internal/gmailauth/store.go`. Define `TokenRecord{AccessToken, RefreshToken, TokenType string, Expiry time.Time}` and `AuthorizationRecord{SchemaVersion int, MailAccountID, MailAccountName string, MailAccountEmailAddresses []string, GmailEmail, OAuthClientID string, Token TokenRecord}` with explicit lower-camel JSON tags. Define `Validate() error` and `Store` methods `Get(context.Context, string) (AuthorizationRecord, error)`, `Put(context.Context, AuthorizationRecord) error`, `Delete(context.Context, string) error`, and `List(context.Context) ([]AuthorizationRecord, error)`. Define `ErrNotFound`, `ErrInvalidRecord`, `ErrUnsupportedPlatform`, and `ErrReauthorizationRequired`; validation must require schema `1`, complete Mail identity, Gmail email, public OAuth client ID, and refresh token. The record must have no client-secret, auth-URL, or token-URL field. | [ ] | |
| TASK-008 | Add `github.com/keybase/go-keychain v0.0.1` to `go.mod` and `go.sum`. Create `internal/gmailauth/keychain_darwin.go` with `//go:build darwin && cgo`, `NewStore() Store` using fixed service `com.alewkinr.mail-app-cli.gmail-oauth`, and unexported `newKeychainStore(service string) Store` for isolated tests. Implement generic-password CRUD/list, account key equal to Mail.app account ID, update-on-duplicate semantics, non-synchronizing while-unlocked accessibility, context checks before OS calls, item-not-found mapping to `ErrNotFound`, and secret-safe OSStatus wrapping. `Put` must validate before serialization; `Get` and every item returned by `List` must validate after decoding and wrap `ErrInvalidRecord` without including serialized record or token data. | [ ] | |
| TASK-009 | Create `internal/gmailauth/keychain_unsupported.go` with `//go:build !darwin || !cgo`. `NewStore` and every Store method must return or implement `ErrUnsupportedPlatform` with a deterministic message naming macOS and `CGO_ENABLED=1`; no file-based fallback is permitted. | [ ] | |
| TASK-010 | Add `pkg/mail/message_identity_test.go`, `internal/gmailauth/store_test.go`, `internal/gmailauth/keychain_darwin_test.go` with `//go:build darwin && cgo`, and `internal/gmailauth/keychain_unsupported_test.go` with `//go:build !darwin || !cgo`. Test resolver script/argv propagation, all resolver error codes and blank-field validation, `errors.Is` sentinels, pure account selection, record JSON/validation/schema errors, Put/Get/List validation enforcement, every unsupported Store method, and runtime-generated secret redaction. Gate real Keychain CRUD with `MAIL_APP_CLI_KEYCHAIN_TEST=1`, use `newKeychainStore` with a random service suffix, and delete items with `t.Cleanup`. | [ ] | |

Dependencies: TASK-002 and TASK-004 depend on TASK-001. TASK-003 depends on TASK-002. TASK-004 depends on TASK-002 and TASK-003. TASK-006 depends on TASK-001 and TASK-005. TASK-008 and TASK-009 depend on TASK-007. TASK-010 depends on TASK-001 through TASK-009.

Completion criteria:

- `go test ./internal/gmailauth ./pkg/mail` passes; `CGO_ENABLED=0 go test ./internal/gmailauth` passes; and `GOOS=linux CGO_ENABLED=0 go test -run '^$' -exec=true ./internal/gmailauth` compiles the non-Darwin path.
- A resolver test decodes local ID, RFC `Message-ID`, Mail account ID, all account email addresses, and mailbox name.
- Authorization-record tests prove the JSON schema contains `oauthClientID` and contains no client-secret or credential-file field.
- `rg -n "json\\.(Marshal|NewEncoder)|os\\.(WriteFile|OpenFile|Create)|fs\\.FileMode" internal/gmailauth` is reviewed against an allowlist: authorization-record JSON encoding may occur only in `keychain_darwin.go`, test code may encode fixtures, and production code contains no file-creation/write path.

### Implementation Phase 2

- GOAL-002: Implement the narrow Gmail REST client and exact single-message archive semantics before OAuth and command wiring depend on it.

| Task | Description | Completed | Date |
| --- | --- | --- | --- |
| TASK-011 | Create `internal/gmailapi/errors.go` with `ErrNotAuthorized`, `ErrMessageNotFound`, `ErrAmbiguousMessage`, and `ErrInvalidResponse`. Add `HTTPError{StatusCode int, RetryAfter time.Duration, Operation string}` whose `Error` and `Unwrap` expose no authorization header, token, request body, or raw response body. Map HTTP 401 to `ErrNotAuthorized`, 404 to `ErrMessageNotFound`, malformed success JSON to `ErrInvalidResponse`, and retain status metadata for 400, 403, 429, and 5xx. Parse both delta-seconds and HTTP-date `Retry-After`. | [ ] | |
| TASK-012 | Create `internal/gmailapi/client.go` with `NewClient(httpClient *http.Client) *Client`, unexported/test constructor `newClient(httpClient *http.Client, baseURL *url.URL) *Client`, production base URL `https://gmail.googleapis.com/gmail/v1`, and a shared response decoder limited to 1 MiB. Define `Profile{EmailAddress string}` and `GetProfile(ctx context.Context) (Profile, error)` using `GET /users/me/profile`; reject blank `emailAddress` with `ErrInvalidResponse`. | [ ] | |
| TASK-013 | Create `internal/gmailapi/message_id.go` with `NormalizeRFCMessageID(string) (string, error)`. Trim whitespace, require non-empty content, remove one surrounding `<...>` pair, reject ASCII controls, and construct the search term as `rfc822msgid:<normalized-value>` through `url.Values`. | [ ] | |
| TASK-014 | Add `(*Client).FindInboxMessageByRFCMessageID(ctx context.Context, rfcMessageID string) (string, error)` using `GET /users/me/messages`, `labelIds=INBOX`, `maxResults=2`, and normalized `q`. Return `ErrMessageNotFound` for zero results. Return `ErrAmbiguousMessage` for any result count greater than one or any non-empty `nextPageToken`. Reject a sole result with blank ID as `ErrInvalidResponse`. | [ ] | |
| TASK-015 | Add `(*Client).RemoveInboxLabel(ctx context.Context, gmailMessageID string) error`. Reject blank input ID. Use `POST /users/me/messages/{url.PathEscape(id)}/modify`, content type `application/json`, and exact body `{"removeLabelIds":["INBOX"]}`. Require a non-empty identical response ID and verify `INBOX` is absent from `labelIds`; otherwise return `ErrInvalidResponse`. | [ ] | |
| TASK-016 | Do not automatically retry Gmail lookup or mutation in version 1. Preserve `Retry-After` metadata for callers, but require a new explicit CLI invocation after 429/503 or an uncertain POST outcome. Ensure all endpoint methods propagate `context.Canceled` and `context.DeadlineExceeded`. | [ ] | |
| TASK-017 | Add `internal/gmailapi/client_test.go` and `internal/gmailapi/message_id_test.go` using `httptest.Server`. Verify profile endpoint/decoding/blank email, URL encoding, `labelIds=INBOX`, `maxResults=2`, zero/one/multiple/next-page/blank-ID lookup, exact modify JSON, blank input, missing/mismatched response ID, unchanged-label rejection, malformed and oversized bodies, both `Retry-After` forms, cancellation, every mapped status, and runtime-generated fixture secrets absent from errors. Assert no mutation request occurs after every lookup failure. | [ ] | |

Dependencies: TASK-012 depends on TASK-011. TASK-014 depends on TASK-011 through TASK-013. TASK-015 and TASK-016 depend on TASK-011 and TASK-012. TASK-017 depends on TASK-011 through TASK-016.

Completion criteria:

- `go test ./internal/gmailapi` passes without Google credentials.
- The client never modifies a message when lookup returns zero, multiple results, a next-page token, or an empty Gmail ID.
- Gmail HTTP errors contain typed status/sentinel information but none of the generated authorization or response-body secrets.

### Implementation Phase 3

- GOAL-003: Implement one-time browser-based authorization, silent token refresh, revocation, and account-management commands using the Phase 2 Gmail profile client.

| Task | Description | Completed | Date |
| --- | --- | --- | --- |
| TASK-018 | Add `golang.org/x/oauth2 v0.24.0` to `go.mod` and `go.sum`; this version declares Go 1.18 and is compatible with the repository's Go 1.21 directive. Use `oauth2.GenerateVerifier`, `oauth2.S256ChallengeFromVerifier`, `oauth2.SetAuthURLParam`, and `oauth2.Config.Exchange`; do not add Google's generated Gmail client library. | [ ] | |
| TASK-019 | Create `internal/gmailauth/app_config.go`. Define `OAuthConfig{ClientID, ClientSecret, AuthURL, TokenURL string}`, constants `https://accounts.google.com/o/oauth2/v2/auth` and `https://oauth2.googleapis.com/token`, link-time variables `appOAuthClientID` and `appOAuthClientSecret`, sentinel `ErrOAuthClientNotConfigured`, `AppOAuthConfig() (OAuthConfig, error)`, and an unexported constructor for tests. Trim and require both Desktop client credentials; callers must never load JSON or a runtime credential file. Update `Makefile` so `build-gmail` and `install-gmail` require `GMAIL_OAUTH_CLIENT_ID` and `GMAIL_OAUTH_CLIENT_SECRET` and inject both through linker flags. Existing ordinary build/test targets may produce a binary whose Gmail login returns `ErrOAuthClientNotConfigured`. | [ ] | |
| TASK-020 | Create `internal/gmailauth/oauth.go`. Define `ProfileClient.GetProfile(context.Context) (gmailapi.Profile, error)`, `ProfileClientFactory.NewProfileClient(context.Context, oauth2.TokenSource) ProfileClient`, and `ListenerFactory.Listen(network, address string) (net.Listener, error)`. Define `Authorizer.Authorize(context.Context, LoginRequest) (AuthorizationRecord, error)`, concrete `oauthAuthorizer`, and `NewAuthorizer(ProfileClientFactory, ListenerFactory) Authorizer`; the authorizer owns both dependencies. Define `LoginRequest{MailAccount mail.Account, OAuth OAuthConfig, Output io.Writer, Timeout time.Duration}`. Build `oauth2.Config` with both injected Desktop client credentials, fixed Google endpoints, endpoint `AuthStyle: oauth2.AuthStyleInParams`, and exactly `[]string{"https://www.googleapis.com/auth/gmail.modify"}`; reject configuration that introduces `https://mail.google.com/` or any additional scope. The client secret must be sent only to the token endpoint, never in the authorization URL. Implement authorization with production timeout two minutes, pre-bound `127.0.0.1:0`, `/oauth2/callback`, 32 random state bytes encoded with base64url, PKCE-S256, offline access, and prompt `consent select_account`. Print the authorization URL to `Output` without launching a browser. The method validates and returns the record but never persists it. | [ ] | |
| TASK-021 | Add production `netListenerFactory` and `NewListenerFactory() ListenerFactory` in `oauth.go`. Print `Open this URL to authorize mail-app-cli:` followed by the URL on its own stdout line and keep the callback listener active. The callback page must say only that the response was received and direct the user back to the terminal; it must not claim that login or Keychain persistence succeeded. | [ ] | |
| TASK-022 | In `oauthAuthorizer.Authorize`, first require exactly one non-empty `state`, decode it from base64url, and compare its bytes with the generated 32-byte state through a focused `crypto/subtle.ConstantTimeCompare` helper; then handle a provider-declared OAuth error; then require exactly one non-empty authorization `code`. Continue using ordinary case-insensitive comparison for account emails. Treat the first `/oauth2/callback` request as terminal; exchange with the exact PKCE `code_verifier` and Desktop client secret using parameter authentication; create a per-login `ProfileClient` through `ProfileClientFactory` using `oauth2.StaticTokenSource(exchangedToken)`; and require the Gmail primary email to match one selected Mail.app account email. Reject a mismatch without an override path. Route initial-exchange `invalid_grant`, a missing refresh token, or a `gmailapi.ErrNotAuthorized` profile result through `NewReauthorizationError` with exact login guidance. Other exchange failures may expose only a validated OAuth error code, never the response description/body. Build the returned schema-1 record without storing the Desktop client secret, validate it, and close the callback server deterministically on every terminal path. | [ ] | |
| TASK-023 | Create `internal/gmailauth/token_source.go` with `NewPersistingTokenSource(ctx context.Context, record AuthorizationRecord, store Store, oauthConfig OAuthConfig) (oauth2.TokenSource, error)` and `NewReauthorizationError(accountName string, cause error) error`. Require the stored OAuth client ID to match the current build configuration and construct refresh requests with the injected Desktop client secret, fixed Google endpoints, parameter authentication, and exact `gmail.modify` scope. Use the passed context for refresh requests, serialize refresh/persistence with a mutex, preserve an existing refresh token when Google's response omits it, and persist changed access/refresh/expiry fields. On `invalid_grant`, a missing refresh token, or client-ID mismatch, return the helper's error with exactly one safely quoted login command. Other failures may expose only a validated OAuth error code. | [ ] | |
| TASK-024 | Create `internal/gmailauth/revoke.go` with `Revoker.Revoke(context.Context, string) error`, `NewGoogleRevoker(httpClient *http.Client) Revoker`, and an unexported `newGoogleRevoker(httpClient *http.Client, endpoint *url.URL) Revoker` test seam. The production constructor uses `https://oauth2.googleapis.com/revoke`. POST the refresh token in an `application/x-www-form-urlencoded` body; never put it in URL/query or errors. Limit response reads to 64 KiB, accept only HTTP 200, and return a secret-safe typed error for every other status. | [ ] | |
| TASK-025 | Create `cmd/gmail.go`. Define consumer-owned interfaces `MailAccountClient.GetAccountByName(string) (*mail.Account, error)`, `AuthorizationStore` with the four `gmailauth.Store` methods, `Authorizer.Authorize(context.Context, gmailauth.LoginRequest) (gmailauth.AuthorizationRecord, error)`, and `ProfileClientFactory.NewProfileClient(context.Context, oauth2.TokenSource) gmailauth.ProfileClient`. Define `gmailCommandDeps{MailAccounts MailAccountClient, Store AuthorizationStore, LoadOAuthConfig func() (gmailauth.OAuthConfig, error), Authorizer Authorizer, Profiles ProfileClientFactory, Revoker gmailauth.Revoker, Stdout io.Writer}` and constructors `newGmailCmd`, `newGmailAuthCmd`, `newGmailAuthLoginCmd`, `newGmailAuthStatusCmd`, and `newGmailAuthLogoutCmd`. Add `gmailProfileClientFactory`, which builds a per-authorization `oauth2.NewClient(ctx, source)` and `gmailapi.NewClient`. `newProductionGmailCommandDeps()` sets `LoadOAuthConfig` to `gmailauth.AppOAuthConfig` and wires native Keychain, `gmailauth.NewAuthorizer(profiles, gmailauth.NewListenerFactory())`, the same profile factory for status, and Google revocation. Register the top-level command in `cmd/root.go:25-33`. | [ ] | |
| TASK-026 | Implement `gmail auth login --account <Mail.app account>` with required `--account` and short alias `-a`. Resolve exactly that Mail.app account before printing the authorization URL, load the compiled app OAuth configuration, call `Authorize` with command stdout, call `record.Validate`, then perform the single owning `Store.Put`. After successful storage, print the Mail account name and Gmail address. Return wrapped `ErrOAuthClientNotConfigured` before printing a URL or writing Keychain when the release lacks either Desktop client credential. No persistence occurs inside `Authorize`. Do not add credential-file, client-secret, or email-mismatch override flags. | [ ] | |
| TASK-027 | Implement `gmail auth status [-a/--account <Mail.app account>]`. With `--account`, resolve the Mail account first; an unlinked account emits one object with its Mail identity, empty `gmailEmail`, `authorized:false`, and `tokenExpiry:null` and exits zero. Without `--account`, call `Store.List`, include linked records only, sort case-insensitively by Mail account name then account ID, and always emit a JSON array (including `[]`). Load the compiled OAuth configuration and pass it to `NewPersistingTokenSource` for each linked record, then validate live profile access. Emit only redacted account/status fields. Any malformed record, configuration error, inaccessible/non-not-found Keychain error, refresh failure, or profile 401 fails the whole command with no partial JSON. | [ ] | |
| TASK-028 | Implement `gmail auth logout --account <Mail.app account> [--revoke]` with required `--account` and short alias `-a`. Without `--revoke`, delete only that account's Keychain record. With `--revoke`, pass the stored refresh token to `Revoker` before deletion; any revocation failure preserves the Keychain record. Print no token or raw provider response. | [ ] | |
| TASK-029 | Add `internal/gmailauth/app_config_test.go`, `oauth_test.go`, `token_source_test.go`, and `revoke_test.go` with injected authorization output, listener and profile fakes, plus token and revocation servers. Test missing/trimmed/injected Desktop client ID and secret, exact printed authorization URL output, callback validation, PKCE, profile matching, refresh persistence, revocation, and redaction. Assert the client secret is absent from the authorization URL, Keychain record, output, and errors; token exchange and refresh must contain both `client_id` and `client_secret` as form parameters and no HTTP Basic Authorization header. Assert only a validated OAuth error code survives provider-error redaction. | [ ] | |
| TASK-030 | Add `cmd/gmail_test.go` with fake dependencies. Assert exact command/flag shapes, including required `login --account` and `logout --account`, optional `status --account`, and `-a` aliases; login's single post-validation write and zero writes after missing app config or every authorization/validation failure; safe single-account object and sorted multi-account array status JSON; empty linked-account list; unlinked versus invalid/inaccessible Keychain behavior; missing-refresh, invalid-grant, and profile-401 login guidance; no partial status output; revocation-before-delete; revocation failure preserving the record; missing-record behavior; and absence of password, app-password, client-secret, credential-file, and email-mismatch override flags. | [ ] | |

Dependencies: TASK-020 depends on TASK-018, TASK-019, Phase 1 Mail types, and Phase 2's `gmailapi.Profile`. TASK-021 depends on the interfaces in TASK-020. TASK-022 depends on TASK-019 through TASK-021 and TASK-023. TASK-023 depends on TASK-018, TASK-019, and Phase 1's authorization record. TASK-024 depends on TASK-018. TASK-025 depends on TASK-019 through TASK-024 and Phase 2's Gmail client. TASK-026 through TASK-028 depend on TASK-025; TASK-027 also depends on TASK-023 and the authenticated profile factory in TASK-025. TASK-029 and TASK-030 depend on TASK-018 through TASK-028.

Completion criteria:

- `go test ./internal/gmailauth ./cmd` passes without opening a real browser, contacting Google, or writing a real Keychain item.
- Login tests prove password, app-password, runtime client-secret, and credential-file inputs do not exist and authorization is stored exactly once after profile validation.
- `make build-gmail` fails when either Desktop client credential is blank; with generated fake values it builds successfully and `AppOAuthConfig` tests prove both injected values are used.
- Refresh, revocation, and HTTP errors preserve typed errors while exposing none of the generated secrets.

### Implementation Phase 4

- GOAL-004: Route linked accounts through Gmail API while retaining Mail.app behavior for unlinked accounts.

| Task | Description | Completed | Date |
| --- | --- | --- | --- |
| TASK-031 | Create `internal/archive/service.go`. Define `MessageResolver.ResolveMessageIdentity(accountName, mailboxName, localMessageID string) (*mail.MessageIdentity, error)`, `MailArchiver.ArchiveMessage(accountName, mailboxName, localMessageID string) error`, `AuthorizationStore.Get(context.Context, string) (gmailauth.AuthorizationRecord, error)`, `TokenClientFactory.NewClient(context.Context, gmailauth.AuthorizationRecord) (GmailClient, error)`, and `GmailClient` methods `FindInboxMessageByRFCMessageID(context.Context, string) (string, error)` and `RemoveInboxLabel(context.Context, string) error`. Define `NewService(MessageResolver, MailArchiver, AuthorizationStore, TokenClientFactory) *Service`. Add `internal/archive/gmail_client_factory.go` with `NewGmailClientFactory(store gmailauth.Store) TokenClientFactory`; its concrete adapter calls `gmailauth.NewPersistingTokenSource(ctx, record, store)`, then `oauth2.NewClient(ctx, source)`, then `gmailapi.NewClient(httpClient)`. | [ ] | |
| TASK-032 | Define `type Backend string`, constants `BackendGmailAPI Backend = "gmail-api"` and `BackendMailApp Backend = "mail-app"`, `Request{AccountName, MailboxName, LocalMessageID string}`, and `Result{Backend Backend}`. Implement `(*Service).Archive(ctx context.Context, request Request) (Result, error)`. Check `ctx.Err()` before each non-context-aware Mail.app resolver or archiver call so already-canceled requests do no Mail.app work; otherwise resolve the local message ID to its identity first. | [ ] | |
| TASK-033 | In `Service.Archive`, call `AuthorizationStore.Get(ctx, identity.AccountID)`. Treat only `errors.Is(storeErr, gmailauth.ErrNotFound)` as unlinked, call Mail.app with the original local message ID unchanged, and return `Result{Backend: BackendMailApp}` after success. For malformed records and every other Store/Keychain error, return a wrapped error with zero Mail.app and Gmail calls. | [ ] | |
| TASK-034 | For a linked record, check for a blank refresh token first and return `gmailauth.NewReauthorizationError(identity.AccountName, cause)` before generic record validation. Then validate the stored record and compare its Mail account ID to the resolved identity before constructing a token source/client or making HTTP calls. After validation, construct the Gmail client, find the unique inbox message by resolved RFC `Message-ID`, remove `INBOX`, and return `Result{Backend: BackendGmailAPI}`. Never fall back after any linked-account error. | [ ] | |
| TASK-035 | Preserve an existing `gmailauth.ErrReauthorizationRequired` error without calling `NewReauthorizationError` again. Map a bare `gmailapi.ErrNotAuthorized` through that helper; invalid grant and missing refresh are already mapped by TASK-023 and TASK-034. Every resulting error must preserve `errors.Is(err, ErrReauthorizationRequired)`, contain exactly one safely quoted `mail-app-cli gmail auth login --account "<account-name>"` command, and include no token/provider response. | [ ] | |
| TASK-036 | In `cmd/messages.go`, define consumer-owned `ArchiveService.Archive(context.Context, archive.Request) (archive.Result, error)`, `archiveCommandDeps{Service ArchiveService, InvalidateMailboxCache func(string,string), Stdout io.Writer}`, `newProductionArchiveCommandDeps() archiveCommandDeps`, and `runArchive(ctx context.Context, deps archiveCommandDeps, account, mailbox, localMessageID string) error`. The production constructor creates one native `gmailauth.Store`, passes it to both `archive.NewService` and `archive.NewGmailClientFactory`, calls with `cmd.Context()`, invalidates source/Archive/All Mail only after success, and prints exactly `Message archived`. Preserve the public command shape `messages archive <local-message-id> -a/--account <account> -m/--mailbox <mailbox>`. | [ ] | |
| TASK-037 | Leave `pkg/mail/client.go:409-429` and `pkg/mail/scripts/archive.scpt` as the unlinked fallback. Update archive help to state that linked Gmail accounts use Gmail API without Accessibility access and unlinked accounts use Mail.app's Archive action with Accessibility access. | [ ] | |
| TASK-038 | Add `internal/archive/service_test.go` and `cmd/messages_test.go`. Cover local-ID-to-RFC-ID resolution, rejection of a blank local ID, exact stable account-ID Store key, original-local-ID Mail fallback, both backend result constants, linked success, arbitrary non-not-found Store failure, malformed/missing-refresh record, account-ID mismatch before downstream calls, zero/multiple Gmail match, invalid grant, revoked/401 authorization, already-canceled context before each Mail.app call, `errors.Is` preservation, exactly one login command in every reauthorization error, public Cobra args/flags, cache invalidation only after success, exact success stdout, and zero success output/invalidation on every failure. Do not add a direct RFC-ID archive-input test path. | [ ] | |

Dependencies: TASK-032 depends on TASK-031. TASK-033 through TASK-035 depend on TASK-031, TASK-032, and Phases 1 through 3. TASK-036 and TASK-037 depend on TASK-033 through TASK-035. TASK-038 depends on TASK-031 through TASK-037.

Completion criteria:

- `go test ./internal/archive ./cmd` passes.
- A linked-account API failure invokes the Mail.app archiver zero times.
- An unlinked account invokes the Gmail client zero times.
- Existing archive stdout remains byte-for-byte compatible.

### Implementation Phase 5

- GOAL-005: Document, validate, and manually verify the feature without exposing credentials.

| Task | Description | Completed | Date |
| --- | --- | --- | --- |
| TASK-039 | Update `README.md` with end-user usage and maintainer build requirements. State that users do not create a Google Cloud project, enable an API, download a credential file, or supply a Gmail password, app password, or OAuth client credential. Document both maintainer-only build environment values plus macOS, native Keychain, and `CGO_ENABLED=1` requirements. | [ ] | |
| TASK-040 | Add `docs/gmail-oauth-security.md` sections `Threat model`, `OAuth scope`, `App OAuth client`, `macOS Keychain`, `Internet Accounts boundary`, `Revocation`, and `Distribution`. Explain exact `gmail.modify` use, the observable Desktop-client-credential/private-user-token distinction, Keychain service `com.alewkinr.mail-app-cli.gmail-oauth`, client-secret redaction boundaries, why `/usr/bin/security` and file fallback are rejected, and how to revoke through CLI and Google Account settings. | [ ] | |
| TASK-041 | In `docs/gmail-oauth-security.md#internet-accounts-boundary`, state that Mail.app's Google token cannot be imported through a supported API, initial CLI authorization is separate, an existing browser session may avoid password entry, and later access is silent until expiry/revocation. Do not describe deprecated Accounts.framework access as a fallback. | [ ] | |
| TASK-042 | In `docs/gmail-oauth-security.md#distribution`, document the maintainer-only setup: provision one Desktop OAuth client, enable Gmail API, configure consent, inject `GMAIL_OAUTH_CLIENT_ID` and `GMAIL_OAUTH_CLIENT_SECRET`, and build releases with `make build-gmail`. State that installed apps cannot keep the Desktop client secret confidential, it is not a user/server-side authorization boundary, and it remains absent from runtime inputs and outputs. Retain testing-mode expiry, restricted-scope verification, Workspace policy, and consistent-signing guidance. | [ ] | |
| TASK-043 | Run formatting, vet, focused/race/full tests, non-CGO and Linux compilation, blank-ID and blank-secret expected-failure builds, a fake-credential successful Gmail build, `git diff --check`, and secret scans. Scans must prove the client secret is used only in app configuration and token form parameters and is absent from authorization URLs, Keychain records, status, errors, and committed values; token-like runtime values must have no matches. | [ ] | |
| TASK-044 | Create `docs/validation/gmail-api-archive.md` and record every TASK-043 command, exit status, and concise output. Record CON-006's exact pre-existing command/error separately. If it remains unresolved, implementation status cannot become `Completed` without a dated, owner-named waiver in this validation file; do not weaken the test. | [ ] | |
| TASK-045 | Run a manual end-to-end test with a maintainer-built release and a dedicated Gmail account: run login with only `--account`, confirm no credential file or client-secret input is requested, confirm authorization only through redacted `gmail auth status` or Keychain Access metadata without reading the secret payload, archive one inbox message by local ID, verify `INBOX` is removed in Gmail and after Mail.app sync, verify all other labels remain, and verify a second archive returns not-found without another mutation. Never invoke `/usr/bin/security`. | [ ] | |
| TASK-046 | Run a manual two-account isolation test with Gmail accounts A and B both linked to their respective Mail.app account IDs. Archive one B message and verify B's token/backend is used, A is untouched, and the B message alone loses `INBOX`. | [ ] | |
| TASK-047 | Run a manual revocation test by revoking the grant in Google Account settings while retaining the local Keychain record. Poll redacted `gmail auth status --account <account>` every 30 seconds for at most 10 minutes until it returns reauthorization-required; only then archive and verify the exact `gmail auth login --account <account>` guidance with no Mail.app fallback. If revocation does not propagate within the window, record the test as inconclusive rather than passed. Reauthorize and verify archive succeeds. | [ ] | |

Dependencies: TASK-039 through TASK-042 depend on Phases 1 through 4. TASK-043 and TASK-044 depend on all code and documentation tasks. TASK-045 through TASK-047 depend on TASK-044 and require user-controlled test credentials.

Completion criteria:

- All new targeted tests and `go vet ./...` pass.
- `go test ./...` passes.
- Manual tests prove exact single-message `INBOX` removal, label preservation, account isolation, propagated revocation behavior, and no password handling.
- `git grep` finds no access token, refresh token, OAuth authorization code, or Gmail password fixture outside deliberately fake unit-test constants.

## 3. Alternatives

- **ALT-001**: Reuse the Google credential from macOS Internet Accounts or Mail.app. Rejected because there is no supported token-export API for an unrelated CLI; Apple's Accounts framework is deprecated and directs new integrations to the provider.
- **ALT-002**: Use Gmail IMAP with XOAUTH2 and `X-GM-LABELS`. Rejected because Gmail IMAP requires the broader `https://mail.google.com/` scope, while Gmail REST supports this operation with `gmail.modify`.
- **ALT-003**: Continue using Mail.app's UI Archive action for Gmail. Retained only as the unlinked-account fallback because it requires Accessibility permission, opens Mail UI, depends on menu localization/state, and is harder to verify exactly.
- **ALT-004**: Use a Google app password. Rejected because it is a reusable mailbox credential and violates SEC-001.
- **ALT-005**: Use a Google service account. Rejected because personal Gmail does not support service-account mailbox access; Google Workspace use would require administrator-controlled domain-wide delegation.
- **ALT-006**: Store OAuth tokens in a JSON config file. Rejected because macOS Keychain provides encrypted storage and access control designed for small secrets.
- **ALT-007**: Use `github.com/zalando/go-keyring` or execute `/usr/bin/security`. Rejected because its macOS backend launches the general-purpose `security` process instead of binding access to the CLI through native Security.framework calls.
- **ALT-008**: Use `google.golang.org/api/gmail/v1`. Rejected for version 1 because only three REST operations are required; a small typed `net/http` client reduces dependency and generated-code surface.
- **ALT-009**: Remove `INBOX` from every Gmail result matching an RFC `Message-ID`. Rejected because duplicated or malformed `Message-ID` values could archive unintended messages; ambiguous matches must fail without mutation.
- **ALT-010**: Fall back to Mail.app when linked Gmail authorization fails. Rejected because it hides authentication/security failures and can produce a mutation through a backend the user did not expect.
- **ALT-011**: Require every user to create a Google Cloud project and pass a downloaded Desktop client JSON file. Rejected because it creates unnecessary setup and secret-file handling; `mail-app-cli` owns one OAuth client and users only grant consent.
- **ALT-012**: Use Gmail API without any registered Google OAuth project or client ID. Rejected because Google requires a registered OAuth client for access to private Gmail data; shifting exchange to a hosted broker would still require that project and would expose user tokens to additional infrastructure.

## 4. Dependencies

- **DEP-001**: `github.com/keybase/go-keychain v0.0.1` for native macOS Security.framework Keychain access.
- **DEP-002**: `golang.org/x/oauth2 v0.24.0` for Authorization Code flow, PKCE helpers, token exchange, and token refresh while retaining Go 1.21 compatibility.
- **DEP-003**: Gmail REST API endpoints `users.getProfile`, `users.messages.list`, and `users.messages.modify`.
- **DEP-004**: One maintainer-owned Google Cloud project with Gmail API enabled, configured consent screen, Desktop OAuth client, and its client ID and client secret available to the release build as `GMAIL_OAUTH_CLIENT_ID` and `GMAIL_OAUTH_CLIENT_SECRET`.
- **DEP-005**: macOS Keychain, an unlocked login session, CGO, a browser able to reach the loopback callback, and outbound HTTPS access to Google.
- **DEP-006**: Existing Mail.app account and message lookup through JXA for account IDs, email addresses, local message IDs, and RFC `Message-ID`.

## 5. Files

- **FILE-001**: `go.mod` — pin OAuth and native Keychain dependencies.
- **FILE-002**: `go.sum` — record dependency checksums.
- **FILE-003**: `cmd/root.go` — register the Gmail command.
- **FILE-004**: `cmd/gmail.go` — implement Gmail authorization command constructors and handlers.
- **FILE-005**: `cmd/gmail_test.go` — verify auth command behavior and output redaction.
- **FILE-006**: `cmd/messages.go` — route archive through the injectable archive service and preserve cache/output behavior.
- **FILE-007**: `cmd/messages_test.go` — verify archive command integration.
- **FILE-008**: `pkg/mail/client.go` — retain `EmailAddress` and add the complete `EmailAddresses` account field.
- **FILE-009**: `pkg/mail/errors.go` — define typed account/mailbox/message lookup errors.
- **FILE-010**: `pkg/mail/account_lookup.go` — select one exact Mail.app account.
- **FILE-011**: `pkg/mail/account_lookup_test.go` — verify zero/one/multiple account selection.
- **FILE-012**: `pkg/mail/message_identity.go` — resolve a Mail.app local message ID to its RFC identity through an injectable JXA seam.
- **FILE-013**: `pkg/mail/message_identity_test.go` — verify resolver argv, decoding, validation, and sentinels.
- **FILE-014**: `pkg/mail/embed.go` — embed the resolver script.
- **FILE-015**: `pkg/mail/scripts/get_accounts_json.scpt` — expose all configured account email addresses.
- **FILE-016**: `pkg/mail/scripts/resolve_message_identity.scpt` — map a Mail.app local message ID to stable identities.
- **FILE-017**: `pkg/mail/scripts_test.go` — validate the resolver script.
- **FILE-018**: `internal/gmailauth/store.go` — define and validate authorization records and Store signatures.
- **FILE-019**: `internal/gmailauth/keychain_darwin.go` — persist authorization records through native Keychain APIs.
- **FILE-020**: `internal/gmailauth/keychain_unsupported.go` — provide deterministic unsupported-platform behavior.
- **FILE-021**: `internal/gmailauth/app_config.go` — load and validate the release-injected public OAuth client ID and fixed Google endpoints.
- **FILE-022**: `internal/gmailauth/oauth.go` — print the browser OAuth URL and execute loopback, state, and PKCE handling.
- **FILE-025**: `internal/gmailauth/token_source.go` — refresh and persist OAuth tokens.
- **FILE-026**: `internal/gmailauth/revoke.go` — revoke Google grants without leaking tokens.
- **FILE-027**: `internal/gmailauth/*_test.go` — unit-test store, Keychain, OAuth, browser, refresh, revocation, and redaction behavior.
- **FILE-028**: `internal/gmailapi/errors.go` — define typed safe Gmail API errors and retry metadata.
- **FILE-029**: `internal/gmailapi/client.go` — call profile, message-list, and message-modify endpoints.
- **FILE-030**: `internal/gmailapi/message_id.go` — normalize RFC `Message-ID` and build search queries.
- **FILE-031**: `internal/gmailapi/client_test.go` — verify REST behavior against `httptest.Server`.
- **FILE-032**: `internal/gmailapi/message_id_test.go` — verify message-ID normalization and rejection.
- **FILE-033**: `internal/archive/service.go` — choose Gmail API or Mail.app backend.
- **FILE-034**: `internal/archive/gmail_client_factory.go` — construct authenticated Gmail clients with persistent token refresh.
- **FILE-035**: `internal/archive/service_test.go` — prove routing, typed errors, and fail-closed behavior.
- **FILE-036**: `README.md` — document setup, build requirements, and command usage.
- **FILE-037**: `docs/gmail-oauth-security.md` — document auth, Internet Accounts, Keychain, scope, distribution, and revocation boundaries.
- **FILE-038**: `docs/validation/gmail-api-archive.md` — record verification commands, results, and any explicit baseline waiver.
- **FILE-039**: `Makefile` — add deterministic Gmail-enabled build/install targets with linker injection.

## 6. Testing

- **TEST-001**: Unit-test Mail.app local-ID lookup and resolved RFC `Message-ID` decoding.
- **TEST-002**: Compile the new embedded JXA resolver on Darwin.
- **TEST-003**: Unit-test authorization-record validation and secret-safe errors.
- **TEST-004**: Run opt-in native Keychain integration tests with isolated, automatically deleted items.
- **TEST-005**: Unit-test app OAuth configuration, missing build configuration, fixed endpoints, and linker-injected client ID behavior.
- **TEST-006**: Unit-test loopback OAuth, state, PKCE, timeout, account verification, and refresh behavior with local HTTP servers.
- **TEST-007**: Unit-test Gmail REST lookup and modify semantics with local HTTP servers.
- **TEST-008**: Unit-test archive backend routing and fail-closed behavior.
- **TEST-009**: Unit-test Cobra flags, output compatibility, cache invalidation, and token redaction.
- **TEST-010**: Run every exact formatting, vet, targeted-test, no-CGO, non-Darwin compile, full-test, diff, and secret-scan command listed in TASK-043.
- **TEST-011**: Manually verify a successful Gmail archive by local Mail.app ID while preserving all non-`INBOX` labels.
- **TEST-012**: Manually verify multi-account isolation, token revocation, reauthorization, and no Mail.app fallback for linked-account failures.

## 7. Risks & Assumptions

- **RISK-001**: Google can classify or change restricted-scope verification requirements. Mitigation: request only `gmail.modify`, document current requirements, and isolate the scope constant.
- **RISK-002**: External OAuth projects in Testing produce seven-day refresh tokens, which conflicts with long-lived silent refresh. Mitigation: document the limitation and require an appropriate production/internal project for durable use.
- **RISK-003**: Unsigned or frequently rebuilt local binaries may trigger additional Keychain access prompts. Mitigation: use native Keychain access, stable service/account names, and document code-signing as a release hardening step.
- **RISK-004**: A message can lack an RFC `Message-ID`, or duplicate messages can share one. Mitigation: fail without mutation on missing, zero-match, or ambiguous identity.
- **RISK-005**: Mail.app can expose an alias while Gmail profile returns the Workspace primary address. Mitigation: compare every Mail account email and reject the link when none matches; the user must select or configure the matching Mail.app account.
- **RISK-006**: Mail.app may not immediately reflect a Gmail API label mutation. Mitigation: treat Gmail's modify response as the mutation authority, invalidate local caches, and document that Mail.app sync can lag.
- **RISK-007**: Concurrent archives for one account can refresh and persist the same token simultaneously. Mitigation: serialize token persistence and preserve the newest non-empty refresh token.
- **RISK-008**: A POST may succeed even if the client loses the response. Mitigation: do not automatically retry; a later attempt safely searches only `INBOX`, returning not-found after a successful archive.
- **RISK-009**: The former embedded AppleScript baseline failure could obscure regressions. Mitigation: CON-006 is repaired without weakening `TestEmbeddedScripts`, and the complete suite passes.
- **RISK-010**: A Desktop OAuth client ID embedded in a public binary is observable and can be copied by another program. Mitigation: use PKCE and state for every authorization, request only `gmail.modify`, monitor the app's OAuth usage, complete Google verification, and rotate the client ID through a new release if abused.
- **ASSUMPTION-001**: Mail.app account IDs remain stable across account renames but may change after account removal/re-addition; reauthorization is acceptable after re-addition.
- **ASSUMPTION-002**: Archiving one CLI-selected message means removing `INBOX` from that message, not every message in the Gmail thread.
- **ASSUMPTION-003**: The maintainer provisions and governs the app's Google Desktop OAuth client; each user only selects an account and grants the requested Gmail scope.
- **ASSUMPTION-004**: Browser authorization is acceptable once per Mail.app account; fully credentialless Gmail mutation is not possible.
- **ASSUMPTION-005**: Gmail API users install a maintainer-built binary containing the app client ID; ordinary unconfigured development builds continue to support non-Gmail functionality but return `ErrOAuthClientNotConfigured` from Gmail login.

## 8. Related Specifications / Further Reading

- [Google OAuth 2.0 for installed applications](https://developers.google.com/identity/protocols/oauth2)
- [Google OAuth 2.0 for iOS and Desktop Apps](https://developers.google.com/identity/protocols/oauth2/native-app)
- [Google OAuth 2.0 security best practices](https://developers.google.com/identity/protocols/oauth2/resources/best-practices)
- [Gmail API scopes](https://developers.google.com/workspace/gmail/api/auth/scopes)
- [Gmail `users.getProfile`](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users/getProfile)
- [Gmail search and filtering](https://developers.google.com/workspace/gmail/api/guides/filtering)
- [Gmail `users.messages.modify`](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/modify)
- [Apple Accounts framework deprecation](https://developer.apple.com/documentation/Accounts)
- [Apple Keychain Services](https://developer.apple.com/documentation/security/keychain-services)
- [Apple TN3137: On Mac keychains](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains)
