function run(argv) {
	const [accountName, mailboxName, messageID] = argv;
	const mail = Application('Mail');
	let result = null;

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const allIds = mbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx >= 0) {
			const msg = mbox.messages.at(targetIdx);
			const toRecipients = [];
			const toRecs = msg.toRecipients();
			for (let t = 0; t < toRecs.length; t++) {
				toRecipients.push(toRecs[t].address());
			}

			const ccRecipients = [];
			const ccRecs = msg.ccRecipients();
			for (let c = 0; c < ccRecs.length; c++) {
				ccRecipients.push(ccRecs[c].address());
			}

			const bccRecipients = [];
			const bccRecs = msg.bccRecipients();
			for (let b = 0; b < bccRecs.length; b++) {
				bccRecipients.push(bccRecs[b].address());
			}

			result = {
				id: String(msg.id()),
				subject: msg.subject() || '',
				sender: msg.sender() || '',
				dateReceived: (msg.dateReceived() || new Date()).toISOString(),
				dateSent: (msg.dateSent() || new Date()).toISOString(),
				read: msg.readStatus(),
				flagged: msg.flaggedStatus(),
				messageSize: msg.messageSize(),
				content: msg.content() || '',
				mailbox: mbox.name(),
				account: acc.name(),
				toRecipients: toRecipients,
				ccRecipients: ccRecipients,
				bccRecipients: bccRecipients
			};
		}
	} catch (e) {
		// Preserve the existing null-result behavior for unavailable messages.
	}

	return JSON.stringify(result);
}
