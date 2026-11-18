function run(argv) {
	const [accountName, localID] = argv;
	const mail = Application('Mail');
	const scope = resolveAccount(mail, accountName);
	if (scope.error) return scope.error;
	const account = scope.account;
	const drafts = resolveSpecialMailbox(mail.draftsMailbox().mailboxes(), account, 'Drafts');
	if (drafts.error) return drafts.error;
	const trash = resolveSpecialMailbox(mail.trashMailbox().mailboxes(), account, 'Trash');
	if (trash.error) return trash.error;

	const source = drafts.mailbox;
	const destination = trash.mailbox;
	const sourceName = source.name();
	const destinationName = destination.name();
	const accountID = String(account.id());
	const result = status => JSON.stringify({status, source: sourceName, destination: destinationName});
	const matches = source.messages.id().filter(id => String(id) === localID);
	if (matches.length === 0) return result('not_found');
	if (matches.length !== 1) return result('ambiguous');
	const draft = source.messages.byId(matches[0]);
	const mailbox = draft.mailbox();
	if (String(draft.id()) !== localID || draft.deletedStatus() ||
	    mailbox.name() !== sourceName || String(mailbox.account().id()) !== accountID) {
		return result('not_found');
	}

	const rfcID = draftRFCID(draft);
	const previousTrashIDs = new Set(destination.messages.id().map(String));
	try {
		draft.mailbox = destination;
	} catch (e) {
		return result('uncertain');
	}

	for (let attempt = 0; attempt < 10; attempt++) {
		if (attempt > 0) delay(0.35);
		try {
			if (source.messages.id().some(id => String(id) === localID)) continue;
			const arrivals = destination.messages.id().filter(id => !previousTrashIDs.has(String(id)));
			const matching = arrivals.filter(id => String(id) === localID ||
				(rfcID !== '' && draftRFCID(destination.messages.byId(id)) === rfcID));
			if (matching.length === 1) return result('moved');
			if (matching.length > 1) return result('uncertain');
		} catch (e) {
			// Mail can expose an ID before the corresponding record is readable.
		}
	}
	return result('uncertain');
}

function draftRFCID(message) {
	try {
		return String(message.messageId() || '').trim();
	} catch (e) {
		return '';
	}
}
