on run argv
	tell application "Mail"
		set mailboxList to {}
		repeat with acc in accounts
			repeat with mbox in mailboxes of acc
				set mailboxInfo to {name:(name of mbox), unreadCount:(unread count of mbox), totalCount:(count of messages in mbox), account:(name of acc)}
				set end of mailboxList to mailboxInfo
			end repeat
		end repeat
		return mailboxList
	end tell
end run
