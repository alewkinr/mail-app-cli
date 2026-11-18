---
worth: yes
where: pkg/mail/scripts/send_message.scpt:53
added: 2026-11-17
---

# Create Mail.app drafts

Add a `draft create` command that saves a new, unsent message to the selected
account's Drafts mailbox. Reuse the existing composition logic for sender,
recipients, subject, body, and attachments, and call Mail.app's `save` command.

Verify in Mail.app that the compose window can close, the message appears
exactly once in `messages drafts` for the selected account, and its contents and
attachments survive reopening. Check whether invisible composition leaves a
background compose session that can recreate a deleted draft.

Explore reply drafts separately using Mail.app's native `reply` followed by
`save`. Verify that the saved draft retains its thread association.
