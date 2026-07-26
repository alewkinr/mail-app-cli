function run(argv) {
	const [accountName, sourceMailbox, messageID, targetMailbox] = argv;
	const mail = Application('Mail');

	try {
		const acc = mail.accounts.byName(accountName);
		const sourceMbox = acc.mailboxes.byName(sourceMailbox);
		const allIds = sourceMbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx < 0) {
			return 'Error: Message not found';
		}

		const destMbox = acc.mailboxes.byName(targetMailbox);
		sourceMbox.messages.at(targetIdx).mailbox = destMbox;
		return 'Success';
	} catch (e) {
		return 'Error: ' + e;
	}
}
