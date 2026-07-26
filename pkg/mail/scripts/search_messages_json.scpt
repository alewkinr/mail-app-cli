function run(argv) {
	const [query, accountName, mailboxName, limitValue] = argv;
	const mail = Application('Mail');
	const result = [];
	const searchTerm = query.toLowerCase();
	const maxResults = Number(limitValue) || 0;

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const accName = acc.name();
		const mboxName = mbox.name();
		const messages = mbox.messages();
		const maxToCheck = Math.min(messages.length, 500);

		for (let k = 0; k < maxToCheck && result.length < maxResults; k++) {
			const msg = messages[k];
			try {
				const subject = (msg.subject() || '').toLowerCase();
				const sender = (msg.sender() || '').toLowerCase();

				if (subject.includes(searchTerm) || sender.includes(searchTerm)) {
					result.push({
						id: String(msg.id()),
						subject: msg.subject() || '',
						sender: msg.sender() || '',
						dateReceived: (msg.dateReceived() || new Date()).toISOString(),
						dateSent: (msg.dateSent() || new Date()).toISOString(),
						read: msg.readStatus(),
						flagged: msg.flaggedStatus(),
						messageSize: msg.messageSize(),
						mailbox: mboxName,
						account: accName
					});
				}
			} catch (e) {
				// Skip messages that cause errors.
			}
		}
	} catch (e) {
		// Preserve the existing empty-result behavior for unavailable mailboxes.
	}

	return JSON.stringify(result);
}
