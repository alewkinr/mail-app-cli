on run argv
	set accountName to item 1 of argv

	tell application "Mail"
		set mailboxList to {}
		try
			set targetAccount to account accountName
			repeat with mbox in mailboxes of targetAccount
				set mailboxInfo to {name:(name of mbox), unreadCount:(unread count of mbox), totalCount:(count of messages in mbox), account:(name of targetAccount)}
				set end of mailboxList to mailboxInfo
			end repeat
		end try
		return mailboxList
	end tell
end run
