on run argv
	tell application "Mail"
		set accountList to {}
		repeat with acc in accounts
			try
				set accountEmailAddresses to email addresses of acc
			on error
				set accountEmailAddresses to ""
			end try
			try
				set accountTypeName to (delivery account of acc) as string
			on error
				set accountTypeName to "unknown"
			end try
			set accountInfo to {id:id of acc, name:name of acc, emailAddress:accountEmailAddresses, accountType:accountTypeName, userName:user name of acc, enabled:enabled of acc}
			set end of accountList to accountInfo
		end repeat
		return accountList
	end tell
end run
