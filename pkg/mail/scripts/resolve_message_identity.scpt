function errorResult(code) {
	return JSON.stringify({error: code});
}

function findMailbox(mailboxes, mailboxName) {
	for (let i = 0; i < mailboxes.length; i++) {
		const mailbox = mailboxes[i];
		if (String(mailbox.name()) === mailboxName) {
			return mailbox;
		}

		const nested = findMailbox(mailbox.mailboxes(), mailboxName);
		if (nested !== null) {
			return nested;
		}
	}

	return null;
}

function run(argv) {
	const accountName = argv[0];
	const mailboxName = argv[1];
	const localMessageID = argv[2];
	const mail = Application('Mail');

	try {
		const accounts = mail.accounts().filter(account =>
			String(account.name()) === accountName
		);
		if (accounts.length === 0) {
			return errorResult('account_not_found');
		}
		if (accounts.length > 1) {
			return errorResult('account_ambiguous');
		}

		const account = accounts[0];
		const mailbox = findMailbox(account.mailboxes(), mailboxName);
		if (mailbox === null) {
			return errorResult('mailbox_not_found');
		}

		const messages = mailbox.messages();
		let message = null;
		for (let i = 0; i < messages.length; i++) {
			if (String(messages[i].id()) === localMessageID) {
				message = messages[i];
				break;
			}
		}
		if (message === null) {
			return errorResult('message_not_found');
		}

		const rfcMessageID = message.messageId();
		if (!rfcMessageID) {
			return errorResult('message_id_missing');
		}

		return JSON.stringify({
			localID: String(message.id()),
			rfcMessageID: String(rfcMessageID),
			accountID: String(account.id()),
			accountName: String(account.name()),
			accountEmailAddresses: account.emailAddresses().map(String),
			mailboxName: String(mailbox.name())
		});
	} catch (error) {
		return errorResult('resolver_failed');
	}
}
