# Unify MailCLI message reads

## Overview

Use one JXA message reader for metadata and batch detail requests. Expose RFC
Message-ID on every message result. Target v3.0.0 for the changed CLI contract.

## Context (from discovery)

`pkg/mail/client.go` has active JXA JSON readers and legacy AppleScript methods
that call a stub parser. The scripts under `pkg/mail/scripts` repeat message
property reads. `cmd/messages.go` and `cmd/search.go` expose those readers.
`MessageIdentity` uses the RFC header for the Gmail archive path.

## Development Approach

Use regular development with focused changes and regression tests in each
code task. Run tests before advancing and update this plan if scope changes.
The agreed CLI changes deliberately break compatibility with v2.1.0.

## Testing Strategy

Cover command arguments, JSON contracts, script compilation, and lookup
failures with automated tests. Verify the Mail.app bridge on macOS.

## Progress Tracking

Check off each completed item. Record blockers and update the plan when scope
changes.

## Solution Overview

Keep separate commands and route them through one embedded JXA script. The
mode selects messages. Shared property-reading code builds the result after
selection.

## Technical Details

`Message` holds metadata including local `id` and nullable `rfcMessageID`.
`MessageDetail` embeds it and adds `content`, recipients, and message size.
Metadata readers never fetch bodies. `show` accepts positional local IDs or
repeatable `--rfc-message-id` flags within one account and source mailbox.
It returns an array in request order, including for one match. Write commands
continue to use local IDs.

## What Goes Where

The checklist covers repository changes and automated verification.
Post-completion notes cover live Mail.app verification and deferred benchmarks.

## Implementation Steps

### Task 1: Establish the lint baseline

**Files**

- Create `.golangci.yml` and `AGENTS.md`.
- Modify `Makefile`.

- [x] Pin a compatible golangci-lint version and start with default checks.
- [x] Add `make lint` and document `make lint` and `go test ./...` after
      feature changes in `AGENTS.md`.
- [x] Document live macOS checks for Mail.app bridge changes.
- [x] Verify the lint and test baseline on macOS.

### Task 2: Consolidate metadata reads

**Files**

- Create `pkg/mail/scripts/read_messages.scpt`.
- Modify `pkg/mail/client.go`, `pkg/mail/embed.go`, `cmd/messages.go`,
  `cmd/search.go`, and the affected message tests.
- Remove the superseded message-reading scripts and legacy parser.

- [x] Route list, inbox, search, special mailbox views, and show through one
      JXA reader. Preserve selection, filtering, pagination, and ordering.
- [x] Split metadata and detail into the two Go types. Read the RFC header
      from each selected Mail.app object and emit JSON null when unavailable.
- [x] Remove `--with-content` and legacy AppleScript readers and parser.
- [x] Update tests for metadata JSON, missing RFC headers, and script
      compilation.
- [x] Run tests and lint.

### Task 3: Support batch show by local or RFC ID

**Files**

- Modify `cmd/messages.go`, `pkg/mail/client.go`,
  `pkg/mail/scripts/read_messages.scpt`, and the affected message tests.

- [x] Accept either positional local IDs or repeatable RFC ID flags.
- [x] Resolve the entire batch before reading bodies. Read local IDs in bulk
      and resolve RFC IDs inside the chosen account and mailbox.
- [x] Fail with a nonzero exit status and empty stdout on any missing or
      ambiguous ID. Report every unresolved ID, its reason, and its mailbox
      scope on stderr. Include matching local IDs for ambiguous RFC IDs.
- [x] Fetch details for successful batches and emit an array in request order.
- [x] Test both selector forms, single-item arrays, rejection of mixed selectors,
      and a batch with missing and ambiguous IDs. Assert zero body reads and
      empty stdout when resolution fails.
- [x] Run tests and lint.

### Task 4: Verify acceptance criteria

**Files**

- Review the changed CLI, scripts, and tests.

- [x] Run `go test ./...` and `make lint`.
- [x] Verify metadata-only output, batch behavior, and unchanged write inputs.

Live macOS checks passed for inbox and list metadata, a two-message batch
with each selector form, and an unsuccessful batch with empty stdout.
Write inputs were checked through tests and source review.

### Task 5: Update documentation and version

**Files**

- Modify `README.md` and `cmd/root.go`.

- [x] Document the metadata and batch detail workflow and removed flag.
- [x] Set the CLI version to 3.0.0 and verify the version output.
- [x] Move this plan to `docs/plans/completed/` when implementation is done.

## Post-Completion

Performance benchmarking remains deferred.
