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

function findMessage(mailbox, messageLocator) {
	const messages = mailbox.messages();
	for (let i = 0; i < messages.length; i++) {
		const message = messages[i];
		if (
			String(message.id()) === messageLocator ||
			message.messageId() === messageLocator
		) {
			return message;
		}
	}

	return null;
}

function archiveThroughMailUI(message) {
	mail.open(message);
	mail.activate();
	delay(0.75);

	const systemEvents = Application('System Events');
	const mailProcess = systemEvents.processes.byName('Mail');
	if (!mailProcess.exists()) {
		throw new Error('Mail process not found');
	}

	const messageMenu = mailProcess.menuBars[0].menus.byName('Message');
	const archiveItem = messageMenu.menuItems.byName('Archive');
	if (!archiveItem.exists()) {
		throw new Error('Mail Archive menu item not found');
	}

	archiveItem.click();
}

function archiveMessage(accountName, sourceMailboxName, messageLocator) {
	try {
		const account = mail.accounts.byName(accountName);
		const sourceMailbox = findMailbox(
			account.mailboxes(),
			sourceMailboxName
		);
		if (!sourceMailbox) {
			return 'Error: Source mailbox not found';
		}

		const targetMessage = findMessage(sourceMailbox, messageLocator);
		if (!targetMessage) {
			return 'Error: Message not found';
		}

		const messageId = targetMessage.messageId();
		if (!messageId) {
			return 'Error: Message-ID not available';
		}

		archiveThroughMailUI(targetMessage);
		delay(0.5);

		if (findMessage(sourceMailbox, messageId)) {
			return 'Error: Archive operation did not remove message from source mailbox';
		}

		return 'Success';
	} catch (e) {
		if (String(e).includes('-1719')) {
			return (
				'Error: Accessibility access is required to use Mail\'s ' +
				'Archive action'
			);
		}
		return 'Error: ' + e;
	}
}

function run(argv) {
	if (argv.length !== 3) {
		return 'Error: Expected account, source mailbox, and message ID';
	}

	return archiveMessage(argv[0], argv[1], argv[2]);
}
