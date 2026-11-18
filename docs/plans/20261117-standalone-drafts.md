# Standalone Mail.app drafts

## Overview

Add `draft list`, `draft show`, `draft create`, and `draft delete`. Remove
`messages drafts` and use `draft list` in its place. Bump the CLI from 3.0.0
to 4.0.0 for the breaking command change. Clients replace a draft by creating
and verifying a new draft, then deleting the old one. Threaded replies remain in
[the backlog](/docs/backlog/threaded-reply-drafts.md).

## Context (from discovery)

`cmd/messages.go` registers the old draft listing. The unified JXA reader
uses `mail.draftMailboxes()`, which returned Mail error `-1708` on the test
Mac. Account-level mailbox enumeration found the saved test drafts.
`pkg/mail/client.go` defines `Message` and `MessageDetail`; its embedded
scripts already handle account scoping and message lookup. The compose
logic in `pkg/mail/scripts/send_message.scpt` provides a reference for
sender and attachment setup.

A live probe saved and closed exactly one standalone draft with only a
body and found both local and RFC IDs. Another probe copied an attachment
byte-for-byte and moved the synthetic original to Trash. These probes used
unique test markers. Identifying a saved draft without those markers and
resolving the actual Drafts mailbox across account types remain open checks.

## Development Approach

Use regular development with focused changes and regression tests for each
code task. Run `go test ./...` and `make lint` on macOS before advancing.
Use temporary live probes for mailbox and save behavior before wiring
commands. Preserve the existing send and message commands apart from the
agreed removal of `messages drafts`.

## Testing Strategy

Use stubbed runners or JXA fixtures for scope resolution, serialization,
ambiguous identity, and failures before and after a save or move. Compile
embedded scripts in the existing script suite. Verify live with synthetic
drafts only and record which account configurations were tested.

## Progress Tracking

Check off completed items as they pass. Record blockers and revise this plan
if the verified Mail.app behavior changes the design.

## Solution Overview

Resolve account-specific Drafts scope once per operation and reuse the shared
message reader and JSON models. Use native Mail.app scripting to compose,
save, close, and reconcile the saved record. Resolve Trash in the same account
before moving a draft.

## Technical Details

-   `draft list [--account <name>]` returns a JSON array of `Message` metadata.
    Without an account, list across configured accounts. Return `[]` for a
    successful empty result. Surface account or folder lookup failures.
-   `draft show <local-id> --account <name>` returns one `MessageDetail`
    object. It reads only from the resolved Drafts folder.
-   `draft create --account <name>` accepts `--subject`, `--body`, `--to`,
    `--cc`, `--bcc`, and `--attach`. A body-only draft is valid. Success
    returns one `Message` with the saved draft's local ID, account, mailbox,
    and nullable `rfcMessageID`.
-   `draft delete <local-id> --account <name>` moves the verified saved draft
    to the account's Trash. A missing or ambiguous account, folder, or draft
    fails before mutation.

Serialize MailCLI creates per account across processes. Under that lock,
snapshot the Drafts IDs before composition, then poll fresh reads after
closing the compose window. Reconcile the save with one newly visible row
that matches the intended draft. An ambiguous result or a timeout leaves
the save uncertain. Mail scripting provides no direct link from a compose
object to the persisted row, so an identical draft saved concurrently by
another client can still be mistaken for the new draft.

If a save may have succeeded but its result is uncertain, exit nonzero and
identify the account and operation stage on stderr. The client can reconcile
with `draft list` before retrying. Apply the same principle to an uncertain
Trash move. Keep successful JSON on stdout and leave stdout empty on failure.

## What Goes Where

The numbered tasks cover implementation, automated checks, and live evidence
needed to proceed. Post-completion notes cover migration for callers.

## Implementation Steps

### Task 1: Verify mailbox and saved-record resolution

**Files**

- Modify `docs/plans/20261117-standalone-drafts.md` with probe results.
- Use temporary AppleScript and JXA files outside the repository.

- [x] Determine how to resolve the actual Drafts and Trash folders within an
      account without guessing from their names. Check account-specific and
      aggregate folder behavior on the available account types.
- [x] Compare the compose ID and any available RFC ID with the persisted row
      before and after close. Test two identical drafts and a body-only
      draft. Verify that post-close reconciliation distinguishes sequential
      duplicates without inserting an identity marker into the draft.
- [x] Record the expected behavior for missing or ambiguous scope and for a
      save that cannot be verified, including the accepted limitation for
      identical concurrent saves by external clients.

Live probe results:

-   Match children of Mail.app's singular `draftsMailbox()` and
    `trashMailbox()` by their owning account ID. Each of the five configured
    accounts had exactly one child for each role. The aggregate Trash also
    exposed a local child with no matching account ID. The account-level
    special-folder selectors were unavailable. Missing or multiple children
    must fail before a scoped mutation.
-   Fresh reads found exactly one new body-only record before and after close
    on iCloud, IMAP, and an account Mail identifies as `unknown`. The
    iCloud and IMAP accounts already contained identical synthetic drafts.
    In these runs, the compose ID differed from the saved row ID, the
    compose object did not expose an RFC ID, and `save` returned no value.
    In the split-process probe, the saved row's local and RFC IDs both
    changed on close. An earlier iCloud run observed a stable local ID,
    so earlier identities are provisional.
-   A post-close snapshot difference plus field verification distinguished
    new rows with identical content under serial testing. Mail provides no
    causal identity if a different client saves an identical draft during
    the same operation. Even a singleton match can be premature if the
    intended draft becomes visible later. Proceed with serialized MailCLI
    creates and document the limitation for external clients.
-   Synthetic moves used the resolved Trash child. The IMAP source could
    remain visible immediately after the move and disappeared on reread.
    Verify both Drafts absence and Trash presence after bounded polling.

### Task 2: Add draft listing and details

**Files**

- Create `cmd/draft.go` and `cmd/draft_test.go`.
- Modify `cmd/messages.go`, `pkg/mail/client.go`, and
  `pkg/mail/scripts/read_messages.scpt`.
- Update `pkg/mail/message_read_test.go` and add draft bridge tests as needed.

- [ ] Add the `draft` group and wire `list` and `show` through the verified
      Drafts resolver and shared JXA message reader.
- [ ] Remove `messages drafts` and retire the failed unified Drafts path.
- [ ] Test all-account and scoped metadata, empty folders, detail output, and
      missing or ambiguous scope and IDs.
- [ ] Run tests and lint.

### Task 3: Save and verify a standalone draft

**Files**

- Modify `cmd/draft.go`, `cmd/draft_test.go`, and `pkg/mail/embed.go`.
- Create `pkg/mail/draft.go`, `pkg/mail/draft_test.go`, and
  `pkg/mail/scripts/create_draft.scpt`.
- Modify `pkg/mail/scripts_test.go`.

- [ ] Add compose flags and validate the account, sender, and attachments.
      Reuse the existing compose conventions and surface attachment errors.
- [ ] Serialize creates per account. Snapshot Drafts before composition,
      save and close, then poll and match the post-close delta. Return
      persisted metadata on a unique match. Report ambiguity or timeout as
      uncertain and invalidate affected message caches.
- [ ] Test body-only and complete drafts, attachment failures, uncertain
      saves, duplicate content, account-level serialization, and nullable
      RFC Message-ID.
- [ ] Run tests and lint.

### Task 4: Move a saved draft to Trash

**Files**

- Modify `cmd/draft.go`, `cmd/draft_test.go`, `pkg/mail/draft.go`,
  `pkg/mail/draft_test.go`, `pkg/mail/embed.go`, and
  `pkg/mail/scripts_test.go`.
- Create `pkg/mail/scripts/delete_draft.scpt`.

- [ ] Resolve Drafts and Trash in the selected account, match the local ID in
      Drafts, and move only that record. Verify the move and invalidate the
      affected caches.
- [ ] Test successful moves, wrong-account and non-draft IDs, unresolved
      Trash, and uncertain outcomes.
- [ ] Run tests and lint.

### Task 5: Verify acceptance criteria

**Files**

- Update this plan with results and the relevant tests if gaps are found.

- [ ] Run `go test ./...` and `make lint`.
- [ ] On macOS, create and show a body-only draft and an attachment draft.
      Verify saved identities, account placement, nullable RFC ID handling,
      and attachment contents. Move the exact test drafts to Trash and
      verify they no longer appear in Drafts.
- [ ] Verify `draft list` and the absence of `messages drafts`.

### Task 6: Bump the major version and document migration

**Files**

- Modify `cmd/root.go`, `README.md`, and this plan.
- Close `docs/backlog/create-mail-drafts.md` when the feature lands.

- [ ] Document all four commands, migration to `draft list`, identity and
      error semantics, and the client-side create-then-delete workflow.
      Explain that concurrent identical saves by another client can cause
      misidentification.
- [ ] Set the CLI version to 4.0.0 and verify `mail-app-cli --version`.
- [ ] Move the completed plan to `docs/plans/completed/`. Retain the threaded
      reply backlog item.

## Post-Completion

Callers that use `messages drafts` should switch to `draft list`.
