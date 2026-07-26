function run(argv) {
	const accountName = argv[0];
	const mail = Application('Mail');
	const result = [];

	try {
		const acc = mail.accounts.byName(accountName);
		const accName = acc.name();
		const mailboxes = acc.mailboxes();
		for (let j = 0; j < mailboxes.length; j++) {
			const mbox = mailboxes[j];
			try {
				let totalCount = 0;
				try {
					totalCount = mbox.messages.count();
				} catch (e) {}
				result.push({
					name: mbox.name(),
					unreadCount: mbox.unreadCount(),
					totalCount: totalCount,
					account: accName
				});
			} catch (e) {
				// Skip mailboxes that cannot be queried at all.
			}
		}
	} catch (e) {
		// Preserve the existing empty-result behavior for unavailable accounts.
	}

	return JSON.stringify(result);
}
