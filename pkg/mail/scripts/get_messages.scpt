on run argv
	set accountName to item 1 of argv
	set mailboxName to item 2 of argv
	set messageLimit to (item 3 of argv) as integer

	tell application "Mail"
		set messageList to {}
		try
			set targetAccount to account accountName
			set targetMailbox to mailbox mailboxName of targetAccount
			set msgCount to count of messages in targetMailbox
			if messageLimit > 0 and msgCount > messageLimit then set msgCount to messageLimit

			repeat with i from 1 to msgCount
				set msg to message i of targetMailbox
				set msgInfo to {subject:(subject of msg), sender:(sender of msg), dateSent:(date sent of msg as string), dateReceived:(date received of msg as string), isRead:(read status of msg), isFlagged:(flagged status of msg), messageSize:(message size of msg)}
				set end of messageList to msgInfo
			end repeat
		end try
		return messageList
	end tell
end run
