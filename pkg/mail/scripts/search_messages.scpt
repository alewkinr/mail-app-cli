on run argv
	set searchQuery to item 1 of argv
	set messageLimit to (item 2 of argv) as integer

	tell application "Mail"
		set messageList to {}
		set foundMessages to (every message whose subject contains searchQuery or sender contains searchQuery or content contains searchQuery)
		set msgCount to count of foundMessages
		if messageLimit > 0 and msgCount > messageLimit then set msgCount to messageLimit

		repeat with i from 1 to msgCount
			set msg to item i of foundMessages
			try
				set msgInfo to {subject:(subject of msg), sender:(sender of msg), dateSent:(date sent of msg as string), dateReceived:(date received of msg as string), isRead:(read status of msg), isFlagged:(flagged status of msg), messageSize:(message size of msg)}
				set end of messageList to msgInfo
			end try
		end repeat
		return messageList
	end tell
end run
