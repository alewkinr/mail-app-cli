const mail = Application('Mail');

function findMailbox(mailboxes, mailboxName) {
	for (let i = 0; i < mailboxes.length; i++) {
		const mailbox = mailboxes[i];
		if (mailbox.name() === mailboxName) {
			return mailbox;
		}

		try {
			const children = mailbox.mailboxes();
			const found = findMailbox(children, mailboxName);
			if (found) {
				return found;
			}
		} catch (e) {}
	}

	return null;
}

function findMessage(mailbox, localMessageId) {
	const messages = mailbox.messages();
	for (let i = 0; i < messages.length; i++) {
		const message = messages[i];
		if (String(message.id()) === localMessageId) {
			return message;
		}
	}

	return null;
}

function findMessageByMessageId(mailbox, messageId) {
	const messages = mailbox.messages();
	for (let i = 0; i < messages.length; i++) {
		const message = messages[i];
		if (message.messageId() === messageId) {
			return message;
		}
	}

	return null;
}

function archiveMessage(accountName, sourceMailboxName, localMessageId) {
	try {
		const account = mail.accounts.byName(accountName);
		const sourceMailbox = findMailbox(
			account.mailboxes(),
			sourceMailboxName
		);
		if (!sourceMailbox) {
			return 'Error: Source mailbox not found';
		}

		const targetMessage = findMessage(sourceMailbox, localMessageId);
		if (!targetMessage) {
			return 'Error: Message not found';
		}

		const messageId = targetMessage.messageId();
		if (!messageId) {
			return 'Error: Message-ID not available';
		}

		const archiveMailbox = findMailbox(account.mailboxes(), 'Archive');
		if (!archiveMailbox) {
			return 'Error: Archive mailbox not found';
		}

		targetMessage.mailbox = archiveMailbox;

		if (findMessageByMessageId(sourceMailbox, messageId)) {
			return 'Error: Archive operation did not remove message from source mailbox';
		}

		return 'Success';
	} catch (e) {
		return 'Error: ' + e;
	}
}

function run(argv) {
	if (argv.length !== 3) {
		return 'Error: Expected account, source mailbox, and message ID';
	}

	return archiveMessage(argv[0], argv[1], argv[2]);
}
