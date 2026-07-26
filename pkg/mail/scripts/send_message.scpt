on run argv
	set argumentIndex to 1
	set accountName to item argumentIndex of argv
	set argumentIndex to argumentIndex + 1
	set messageSubject to item argumentIndex of argv
	set argumentIndex to argumentIndex + 1
	set messageBody to item argumentIndex of argv
	set argumentIndex to argumentIndex + 1

	set toAddresses to {}
	set toCount to (item argumentIndex of argv) as integer
	set argumentIndex to argumentIndex + 1
	if toCount > 0 then
		repeat toCount times
			set end of toAddresses to item argumentIndex of argv
			set argumentIndex to argumentIndex + 1
		end repeat
	end if

	set ccAddresses to {}
	set ccCount to (item argumentIndex of argv) as integer
	set argumentIndex to argumentIndex + 1
	if ccCount > 0 then
		repeat ccCount times
			set end of ccAddresses to item argumentIndex of argv
			set argumentIndex to argumentIndex + 1
		end repeat
	end if

	set bccAddresses to {}
	set bccCount to (item argumentIndex of argv) as integer
	set argumentIndex to argumentIndex + 1
	if bccCount > 0 then
		repeat bccCount times
			set end of bccAddresses to item argumentIndex of argv
			set argumentIndex to argumentIndex + 1
		end repeat
	end if

	set attachmentPaths to {}
	set attachmentCount to (item argumentIndex of argv) as integer
	set argumentIndex to argumentIndex + 1
	if attachmentCount > 0 then
		repeat attachmentCount times
			set end of attachmentPaths to item argumentIndex of argv
			set argumentIndex to argumentIndex + 1
		end repeat
	end if

	tell application "Mail"
		try
			set targetAccount to account accountName
			set newMessage to make new outgoing message with properties {subject:messageSubject, content:messageBody, visible:false}

			tell newMessage
				set sender to (item 1 of (email addresses of targetAccount as list))

				repeat with recipientAddress in toAddresses
					make new to recipient at end of to recipients with properties {address:(contents of recipientAddress)}
				end repeat

				repeat with recipientAddress in ccAddresses
					make new cc recipient at end of cc recipients with properties {address:(contents of recipientAddress)}
				end repeat

				repeat with recipientAddress in bccAddresses
					make new bcc recipient at end of bcc recipients with properties {address:(contents of recipientAddress)}
				end repeat

				repeat with attachmentPath in attachmentPaths
					try
						make new attachment with properties {file name:(contents of attachmentPath)} at after the last paragraph
					on error
						-- Skip files that cannot be attached.
					end try
				end repeat

				send
			end tell
			return "Success"
		on error errMsg
			return "Error: " & errMsg
		end try
	end tell
end run
