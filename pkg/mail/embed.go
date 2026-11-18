package mail

import _ "embed"

var (
	//go:embed scripts/message_scope.scpt
	messageScopeJXAScript string

	//go:embed scripts/read_messages.scpt
	readMessagesJXAScript string

	//go:embed scripts/get_accounts.scpt
	getAccountsAppleScript string

	//go:embed scripts/get_mailboxes.scpt
	getMailboxesAppleScript string

	//go:embed scripts/get_all_mailboxes.scpt
	getAllMailboxesAppleScript string

	//go:embed scripts/mark_message_read.scpt
	markMessageReadJXAScript string

	//go:embed scripts/flag_message.scpt
	flagMessageJXAScript string

	//go:embed scripts/delete_message.scpt
	deleteMessageJXAScript string

	//go:embed scripts/send_message.scpt
	sendMessageAppleScript string

	//go:embed scripts/get_unread_count.scpt
	getUnreadCountAppleScript string

	//go:embed scripts/get_accounts_json.scpt
	getAccountsJXAScript string

	//go:embed scripts/sync_account.scpt
	syncAccountAppleScript string

	//go:embed scripts/sync_all_accounts.scpt
	syncAllAccountsAppleScript string

	//go:embed scripts/get_mailboxes_json.scpt
	getMailboxesJXAScript string

	//go:embed scripts/archive.scpt
	archiveJXAScript string

	//go:embed scripts/move_message.scpt
	moveMessageJXAScript string

	//go:embed scripts/get_attachments_json.scpt
	getAttachmentsJXAScript string

	//go:embed scripts/save_attachment.scpt
	saveAttachmentJXAScript string

	//go:embed scripts/resolve_message_identity.scpt
	resolveMessageIdentityJXAScript string
)
