# Gmail OAuth security

## Threat model

The Gmail integration protects reusable access and refresh tokens from
accidental disclosure through command output, error messages, logs, process
arguments, and plaintext files. It also prevents one Mail.app account from
using another account's authorization and prevents ambiguous Gmail search
results from mutating multiple messages.

OAuth does not protect a Mac account that is already compromised or an
unlocked Keychain available to malicious software running as that user. The
CLI therefore keeps the authorization surface narrow, fails closed on
Keychain errors, and never falls back to Mail.app after a linked-account Gmail
API failure.

## OAuth scope

The CLI requests exactly:

```text
https://www.googleapis.com/auth/gmail.modify
```

Google classifies that scope as
[restricted](https://developers.google.com/workspace/gmail/api/auth/scopes).
It is used to read the minimal message identity and label metadata needed for
lookup and to remove the `INBOX` label. The broader
`https://mail.google.com/` scope is not requested. Archiving performs one
unique inbox lookup and one
[`users.messages.modify`](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/modify)
call; duplicated results or pagination fail without mutation.

## App OAuth client

`mail-app-cli` uses one maintainer-owned Desktop OAuth client. Its client ID and
Google-issued Desktop client secret are application metadata embedded in
Gmail-enabled release binaries. Google notes that
[installed applications cannot keep this value confidential](https://developers.google.com/identity/protocols/oauth2/native-app),
so the client secret is not an authorization boundary and must not be confused
with a Gmail password, user OAuth token, or server-side secret.
Users do not provide either client credential or download a credential file.

Desktop authorization uses Authorization Code with PKCE-S256 and a temporary
loopback callback. The CLI prints the authorization URL to stdout and waits;
it never launches a browser. The callback page reports only that the response
was received because profile validation and Keychain persistence still happen
in the terminal process. Token exchange and refresh send both Desktop client
credentials as form parameters and never use HTTP Basic authentication. The
client secret is injected only into maintainer builds, is absent from
authorization URLs, Keychain authorization records, logs, status output, and
errors, and has no runtime flag or credential-file input.

## macOS Keychain

Each complete authorization record is stored as a native generic-password item
under Keychain service:

```text
com.alewkinr.mail-app-cli.gmail-oauth
```

The Keychain account key is the stable Mail.app account ID—not the display
name or email address. Records are non-synchronizing and accessible only while
the user's session is unlocked. The record binds that key to the Mail.app
account name and all email addresses, the verified Gmail primary address, the
public OAuth client ID, and token material.

The implementation calls Security.framework through native Go bindings. It
does not invoke `/usr/bin/security`, because a general-purpose subprocess
would weaken process-bound access and increase argument/output exposure. It
also has no JSON credential-file fallback: if native Keychain access is
unavailable or returns an unexpected error, Gmail operations fail closed.
Errors contain typed status information but omit serialized records, tokens,
authorization codes, authorization headers, and raw provider response bodies.
When macOS reports that the login Keychain is unavailable, the error directs
the user to lock and unlock the `login` Keychain in Keychain Access before
retrying; a successful OAuth callback is not treated as a stored login.

## Internet Accounts boundary

The Google authorization already used by macOS Internet Accounts and Mail.app
cannot be imported by an unrelated CLI through a supported API. Initial
`mail-app-cli` authorization is therefore separate.

If the browser already has an active Google session, the user may be able to
select the intended account and consent without entering a password again.
After that one-time authorization, access-token refresh is silent until the
grant expires, is revoked, or becomes invalid.

## Revocation

Remove only the local Keychain record:

```bash
mail-app-cli gmail auth logout --account "Gmail"
```

Revoke the Google refresh token first, then delete the local record:

```bash
mail-app-cli gmail auth logout --account "Gmail" --revoke
```

If Google revocation fails, the Keychain record is preserved so the user can
retry and the local state does not falsely imply successful revocation. Users
can also revoke access from their Google Account's third-party connections
settings. A revoked or expired linked authorization produces one explicit
`gmail auth login --account` command and never silently falls back to
Mail.app.

## Distribution

Maintainers provision one Google Cloud project, enable the Gmail API,
configure the OAuth consent screen, and create a Desktop OAuth client. Both
Desktop client credentials are injected only while building a Gmail-enabled
release:

```bash
GMAIL_OAUTH_CLIENT_ID=your-desktop-client-id.apps.googleusercontent.com \
GMAIL_OAUTH_CLIENT_SECRET=your-desktop-client-secret \
make build-gmail
```

For external apps in Testing mode, Google documents that authorizations and
offline refresh tokens for scopes such as `gmail.modify`
[expire after seven days](https://developers.google.com/identity/protocols/oauth2#expiration).
The scope is restricted; public distribution requires Google's OAuth
verification, while internal Google Workspace applications remain subject to
their organization's administrator policy.

Production binaries should be built and signed consistently to reduce
unexpected Keychain access prompts and keep the application identity stable.
Release procedures must treat user access and refresh tokens as private even
though both embedded Desktop client credentials are observable.
