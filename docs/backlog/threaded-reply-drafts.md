---
worth: yes
added: 2026-11-17
---

# Support threaded reply drafts

Add `draft reply` to save an unsent reply to an existing message selected by
local or RFC Message-ID. Preserve Mail.app's native subject, recipients,
signature, formatted quote, and thread headers. Insert only the caller's text.

Live tests verified that Mail.app can save a reply with matching
`In-Reply-To` and `References`. Scripted body edits lost quote content or
flattened its formatting. The Accessibility probe could identify the compose
window but could not reliably focus its body editor.

Revisit when an insertion method can preserve the rendered quote. Acceptance
requires a saved, editable draft with the caller's text above the intact quote,
no visible CSS, and both thread headers pointing to the original.
