function run(argv) {
	const [accountName, sourceMailbox, messageID, targetMailbox] = argv;
	const mail = Application('Mail');

	const scope = resolveAccount(mail, accountName);
	if (scope.error) return scope.error;
	const source = resolveMailbox(scope.account, accountName, sourceMailbox);
	if (source.error) return source.error;
	const destination = resolveMailbox(scope.account, accountName, targetMailbox);
	if (destination.error) return destination.error;
	const sourceMbox = source.mailbox;

	try {
		const allIds = sourceMbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx < 0) {
			return 'Error: Message not found';
		}

		sourceMbox.messages.at(targetIdx).mailbox = destination.mailbox;
		return 'Success';
	} catch (e) {
		return 'Error: ' + e;
	}
}
