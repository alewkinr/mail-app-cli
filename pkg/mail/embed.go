package mail

import _ "embed"

var (
	//go:embed scripts/get_accounts.scpt
	getAccountsAppleScript string

	//go:embed scripts/get_mailboxes.scpt
	getMailboxesAppleScript string

	//go:embed scripts/get_all_mailboxes.scpt
	getAllMailboxesAppleScript string

	//go:embed scripts/get_messages.scpt
	getMessagesAppleScript string

	//go:embed scripts/search_messages.scpt
	searchMessagesAppleScript string

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

	//go:embed scripts/get_messages_json.scpt
	getMessagesJXAScript string

	//go:embed scripts/get_message_details_json.scpt
	getMessageDetailsJXAScript string

	//go:embed scripts/archive.scpt
	archiveJXAScript string

	//go:embed scripts/move_message.scpt
	moveMessageJXAScript string

	//go:embed scripts/get_attachments_json.scpt
	getAttachmentsJXAScript string

	//go:embed scripts/save_attachment.scpt
	saveAttachmentJXAScript string

	//go:embed scripts/search_messages_json.scpt
	searchMessagesJXAScript string

	//go:embed scripts/get_special_mailbox_json.scpt
	getSpecialMailboxJXAScript string
)
