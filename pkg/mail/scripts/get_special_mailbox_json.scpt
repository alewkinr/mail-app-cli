function run(argv) {
	const mailboxType = argv[0];
	const perLimit = Number(argv[1]) || 0;
	const withContent = argv[2] === 'true';
	const mail = Application('Mail');
	const result = [];
	let mailboxes;

	switch (mailboxType) {
	case 'sent':
		mailboxes = mail.sentMailboxes();
		break;
	case 'drafts':
		mailboxes = mail.draftMailboxes();
		break;
	case 'trash':
		mailboxes = mail.trashMailboxes();
		break;
	case 'junk':
		mailboxes = mail.junkMailboxes();
		break;
	default:
		return JSON.stringify(result);
	}

	for (let m = 0; m < mailboxes.length; m++) {
		const mbox = mailboxes[m];
		let accName = '';
		let mboxName = '';
		try {
			accName = mbox.account().name();
		} catch (e) {
			try {
				accName = mbox.account.name();
			} catch (e2) {
				accName = '';
			}
		}
		try {
			mboxName = mbox.name();
		} catch (e) {
			mboxName = mailboxType;
		}

		let messages;
		try {
			messages = mbox.messages();
		} catch (e) {
			continue;
		}

		const cap = Math.min(messages.length, perLimit);
		for (let k = 0; k < cap; k++) {
			const msg = messages[k];
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
			} catch (e) {}
		}
	}

	return JSON.stringify(result);
}
