function run(argv) {
	const [accountName, mailboxName, messageID] = argv;
	const mail = Application('Mail');
	const result = [];

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const allIds = mbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx >= 0) {
			const attachments = mbox.messages.at(targetIdx).mailAttachments();
			for (let a = 0; a < attachments.length; a++) {
				const att = attachments[a];
				let mimeType = 'unknown';
				try {
					mimeType = att.mimeType() || 'unknown';
				} catch (e) {
					// mimeType() sometimes fails in Mail.app.
				}
				result.push({
					name: att.name(),
					fileSize: att.fileSize(),
					mimeType: mimeType
				});
			}
		}
	} catch (e) {
		// Preserve the existing empty-result behavior for unavailable messages.
	}

	return JSON.stringify(result);
}
