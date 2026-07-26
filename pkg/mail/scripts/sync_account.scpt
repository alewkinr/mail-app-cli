on run argv
	set accountName to item 1 of argv

	tell application "Mail"
		set accountFound to false
		repeat with acc in accounts
			if name of acc is accountName then
				set accountFound to true
				exit repeat
			end if
		end repeat
		if not accountFound then
			error "Account not found: " & accountName
		end if
	end tell
end run
