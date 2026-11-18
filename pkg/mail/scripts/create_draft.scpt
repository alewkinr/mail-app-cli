on run argv
	set senderAddress to item 1 of argv
	set messageSubject to item 2 of argv
	set messageBody to item 3 of argv
	set argumentIndex to 4
	set toAddresses to my readList(argv, argumentIndex)
	set argumentIndex to argumentIndex + 1 + (count toAddresses)
	set ccAddresses to my readList(argv, argumentIndex)
	set argumentIndex to argumentIndex + 1 + (count ccAddresses)
	set bccAddresses to my readList(argv, argumentIndex)
	set argumentIndex to argumentIndex + 1 + (count bccAddresses)
	set attachmentPaths to my readList(argv, argumentIndex)

	tell application "Mail"
		with timeout of 15 seconds
			set draftMessage to missing value
			try
				set draftMessage to make new outgoing message with properties {sender:senderAddress, subject:messageSubject, content:messageBody, visible:true}
				repeat with recipientAddress in toAddresses
					make new to recipient at end of to recipients of draftMessage with properties {address:(contents of recipientAddress)}
				end repeat
				repeat with recipientAddress in ccAddresses
					make new cc recipient at end of cc recipients of draftMessage with properties {address:(contents of recipientAddress)}
				end repeat
				repeat with recipientAddress in bccAddresses
					make new bcc recipient at end of bcc recipients of draftMessage with properties {address:(contents of recipientAddress)}
				end repeat
			on error
				return my discardPreSaveComposer(draftMessage, "compose failed")
			end try
			try
				repeat with attachmentPath in attachmentPaths
					tell content of draftMessage to make new attachment with properties {file name:(POSIX file (contents of attachmentPath))} at after the last paragraph
				end repeat
			on error
				return my discardPreSaveComposer(draftMessage, "attach failed")
			end try
			try
				-- Mail loads attachments asynchronously before persisting the draft.
				if (count attachmentPaths) > 0 then delay 1
				save draftMessage
			on error
				return "save failed"
			end try
			try
				-- Mail can return from save before the new draft is persisted.
				delay 1
				close draftMessage
			on error
				return "close failed"
			end try
		end timeout
	end tell
	return "saved and closed"
end run

on readList(argv, startIndex)
	set itemCount to (item startIndex of argv) as integer
	if itemCount is 0 then return {}
	return items (startIndex + 1) thru (startIndex + itemCount) of argv
end readList

on discardPreSaveComposer(draftMessage, stage)
	if draftMessage is missing value then return stage & "; composer cleanup unverified"
	tell application "Mail"
		with timeout of 15 seconds
			try
				set draftID to id of draftMessage
				close outgoing message id draftID saving no
				if not (exists outgoing message id draftID) then return stage & "; composer discarded"
			end try
		end timeout
	end tell
	return stage & "; composer cleanup unverified"
end discardPreSaveComposer
