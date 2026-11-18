// A scope failure is returned separately from the command's successful payload.
function scopeFailure(code, account, mailbox) {
	return 'MailCLI scope error: ' + JSON.stringify({code, account, mailbox});
}

function resolveAccount(mail, accountName) {
	const accounts = mail.accounts().filter(account => account.name() === accountName);
	if (accounts.length === 0) {
		return {error: scopeFailure('account_not_found', accountName)};
	}
	if (accounts.length > 1) {
		return {error: scopeFailure('account_ambiguous', accountName)};
	}
	return {account: accounts[0]};
}

function resolveMailbox(account, accountName, mailboxName) {
	const mailbox = account.mailboxes().find(mailbox => mailbox.name() === mailboxName);
	if (!mailbox) {
		return {error: scopeFailure('mailbox_not_found', accountName, mailboxName)};
	}
	return {mailbox};
}

function resolveMessageScope(mail, accountName, mailboxName) {
	const scope = resolveAccount(mail, accountName);
	if (scope.error) return scope;
	return resolveMailbox(scope.account, accountName, mailboxName);
}

// The aggregate mailbox contains the actual mailbox for each configured account.
function resolveSpecialMailbox(children, account, mailboxName) {
	const accountID = String(account.id());
	const accountName = account.name();
	const matches = children.filter(mailbox => {
		const owner = mailbox.account();
		return owner && String(owner.id()) === accountID;
	});
	if (matches.length === 0) {
		return {error: scopeFailure('mailbox_not_found', accountName, mailboxName)};
	}
	if (matches.length > 1) {
		return {error: scopeFailure('mailbox_ambiguous', accountName, mailboxName)};
	}
	return {mailbox: matches[0]};
}
