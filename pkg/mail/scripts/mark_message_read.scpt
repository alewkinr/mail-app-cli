function run(argv) {
	const [accountName, mailboxName, messageID, readValue] = argv;
	const mail = Application('Mail');

	const scope = resolveMessageScope(mail, accountName, mailboxName);
	if (scope.error) return scope.error;
	const mbox = scope.mailbox;

	try {
		const allIds = mbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx < 0) {
			return 'Error: Message not found';
		}

		mbox.messages.at(targetIdx).readStatus = readValue === 'true';
		return 'Success';
	} catch (e) {
		return 'Error: ' + e;
	}
}
