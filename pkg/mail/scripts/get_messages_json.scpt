function run(argv) {
	const accountName = argv[0];
	const mailboxName = argv[1];
	const limit = Number(argv[2]) || 0;
	const offset = Number(argv[3]) || 0;
	const unreadOnly = argv[4] === 'true';
	const flaggedOnly = argv[5] === 'true';
	const withContent = argv[6] === 'true';
	const since = argv[7];
	const mail = Application('Mail');
	const result = [];

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const accName = acc.name();
		const mboxName = mbox.name();
		const messages = mbox.messages();

		// Filtering operates on indices so properties can be fetched in bulk.
		let indices = Array.from({ length: messages.length }, (_, i) => i);

		if (unreadOnly) {
			const readStatuses = mbox.messages.readStatus();
			indices = indices.filter(i => !readStatuses[i]);
		}

		if (flaggedOnly) {
			const flaggedStatuses = mbox.messages.flaggedStatus();
			indices = indices.filter(i => flaggedStatuses[i]);
		}

		if (since) {
			const sinceDate = new Date(since);
			const allDates = mbox.messages.dateReceived();
			indices = indices.filter(i => {
				const date = allDates[i];
				return date && date >= sinceDate;
			});
		}

		if (offset > 0 && indices.length > offset) {
			indices = indices.slice(offset);
		}

		if (limit > 0 && indices.length > limit) {
			indices = indices.slice(0, limit);
		}

		for (let k = 0; k < indices.length; k++) {
			const msg = messages[indices[k]];
			try {
				if (msg.deletedStatus()) {
					continue;
				}
			} catch (e) {}

			try {
				result.push({
					id: String(msg.id()),
					subject: msg.subject() || '',
					sender: msg.sender() || '',
					dateReceived: (msg.dateReceived() || new Date()).toISOString(),
					dateSent: (msg.dateSent() || new Date()).toISOString(),
					read: msg.readStatus(),
					flagged: msg.flaggedStatus(),
					messageSize: 0,
					content: withContent ? (msg.content() || '') : '',
					mailbox: mboxName,
					account: accName
				});
			} catch (e) {
				// Skip messages that cause errors.
			}
		}
	} catch (e) {
		// Preserve the existing empty-result behavior for unavailable mailboxes.
	}

	return JSON.stringify(result);
}
