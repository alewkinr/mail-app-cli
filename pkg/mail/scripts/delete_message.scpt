function run(argv) {
	const [accountName, mailboxName, messageID] = argv;
	const mail = Application('Mail');

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const allIds = mbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx < 0) {
			return 'Error: Message not found';
		}

		mbox.messages.at(targetIdx).delete();
		return 'Success';
	} catch (e) {
		return 'Error: ' + e;
	}
}
