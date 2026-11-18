function run(argv) {
	const [mode, ...args] = argv;
	const mail = Application('Mail');
	switch (mode) {
	case 'draft-list':
		return encodeMessageRead(listDrafts(mail, args[0]));
	case 'draft-mailbox-info':
		return encodeMessageRead(getDraftMailboxAndSender(mail, args[0]));
	case 'draft-get-candidates':
		return encodeMessageRead(getDraftCandidates(mail, args));
	case 'draft-show':
		return encodeMessageRead(showDraft(mail, args[0], args[1]));
	default:
		throw new Error('Unknown draft read mode: ' + mode);
	}
}

function listDrafts(mail, accountName) {
	let accounts;
	if (accountName) {
		const scope = resolveAccount(mail, accountName);
		if (scope.error) return scope.error;
		accounts = [scope.account];
	} else {
		accounts = mail.accounts();
	}
	if (accounts.length === 0) return [];

	const children = mail.draftsMailbox().mailboxes();
	const names = new Set();
	const result = [];
	for (const account of accounts) {
		const name = account.name();
		if (names.has(name)) return scopeFailure('account_ambiguous', name);
		names.add(name);
		const scope = resolveSpecialMailbox(children, account, 'Drafts');
		if (scope.error) return scope.error;
		const mailboxName = scope.mailbox.name();
		for (const message of scope.mailbox.messages()) {
			if (!message.deletedStatus()) result.push(messageMetadata(message, name, mailboxName));
		}
	}
	return result;
}

function showDraft(mail, accountName, localID) {
	const accountScope = resolveAccount(mail, accountName);
	if (accountScope.error) return accountScope.error;
	const scope = resolveSpecialMailbox(mail.draftsMailbox().mailboxes(), accountScope.account, 'Drafts');
	if (scope.error) return scope.error;
	return readMessageDetails(scope.mailbox, accountName, scope.mailbox.name(), 'local', [localID], true);
}

function getDraftMailboxAndSender(mail, accountName) {
	const accountScope = resolveAccount(mail, accountName);
	if (accountScope.error) return accountScope.error;
	const account = accountScope.account;
	const scope = resolveSpecialMailbox(mail.draftsMailbox().mailboxes(), account, 'Drafts');
	if (scope.error) return scope.error;
	const sender = account.emailAddresses()[0] || '';
	const senderKey = sender.toLowerCase();
	const owners = mail.accounts().filter(candidate =>
		candidate.emailAddresses().some(address => address.toLowerCase() === senderKey)
	);
	if (sender && owners.length !== 1) throw new Error('draft sender belongs to multiple accounts');
	return {accountID: String(account.id()), mailbox: scope.mailbox.name(), sender};
}

function getDraftCandidates(mail, args) {
	const [accountName, accountID, mailboxName, subject] = args;
	const accountScope = resolveAccount(mail, accountName);
	if (accountScope.error) return accountScope.error;
	if (String(accountScope.account.id()) !== accountID) throw new Error('draft account changed');
	const scope = resolveSpecialMailbox(mail.draftsMailbox().mailboxes(), accountScope.account, 'Drafts');
	if (scope.error) return scope.error;
	if (scope.mailbox.name() !== mailboxName) throw new Error('draft mailbox changed');

	const drafts = [];
	let pending = false;
	for (const id of scope.mailbox.messages.id()) {
		try {
			const message = scope.mailbox.messages.byId(id);
			if (message.deletedStatus() || (message.subject() || '') !== subject) continue;
			const detail = Object.assign(messageMetadata(message, accountName, mailboxName), messageRecipients(message));
			if (detail.id !== String(id) || String(message.mailbox().account().id()) !== accountID) {
				pending = true;
				continue;
			}
			drafts.push(detail);
		} catch (e) {
			// The ID can be visible before its record properties are readable.
			pending = true;
		}
	}
	return {drafts, pending};
}
