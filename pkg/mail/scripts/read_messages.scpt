function run(argv) {
	const [mode, ...args] = argv;
	const mail = Application('Mail');
	switch (mode) {
	case 'list':
		return encodeMessageRead(listMessages(mail, args));
	case 'search':
		return encodeMessageRead(searchMessages(mail, args));
	case 'special':
		return JSON.stringify(specialMailboxMessages(mail, args));
	case 'show':
		return encodeMessageRead(showMessages(mail, args));
	default:
		throw new Error('Unknown message read mode: ' + mode);
	}
}

function isDeleted(message) {
	try {
		return message.deletedStatus();
	} catch (e) {
		return false;
	}
}

function listMessages(mail, args) {
	const [accountName, mailboxName, limitValue, offsetValue, unreadOnly, flaggedOnly, since] = args;
	const limit = Number(limitValue) || 0;
	const offset = Number(offsetValue) || 0;
	const result = [];

	const scope = resolveMessageScope(mail, accountName, mailboxName);
	if (scope.error) return scope.error;
	const mailbox = scope.mailbox;

	try {
		const messages = mailbox.messages();
		// Filtering operates on indices so properties can be fetched in bulk.
		let indices = Array.from({length: messages.length}, (_, i) => i);

		if (unreadOnly === 'true') {
			const readStatuses = mailbox.messages.readStatus();
			indices = indices.filter(i => !readStatuses[i]);
		}
		if (flaggedOnly === 'true') {
			const flaggedStatuses = mailbox.messages.flaggedStatus();
			indices = indices.filter(i => flaggedStatuses[i]);
		}
		if (since) {
			const sinceDate = new Date(since);
			const allDates = mailbox.messages.dateReceived();
			indices = indices.filter(i => allDates[i] && allDates[i] >= sinceDate);
		}
		if (offset > 0 && indices.length > offset) {
			indices = indices.slice(offset);
		}
		if (limit > 0 && indices.length > limit) {
			indices = indices.slice(0, limit);
		}

		for (const index of indices) {
			const message = messages[index];
			if (isDeleted(message)) {
				continue;
			}
			try {
				result.push(messageMetadata(message, accountName, mailboxName));
			} catch (e) {
				// Skip inaccessible rows.
			}
		}
	} catch (e) {
		// Unavailable mailboxes produce an empty result.
	}

	return result;
}

function searchMessages(mail, args) {
	const [query, accountName, mailboxName, limitValue] = args;
	const result = [];
	const searchTerm = query.toLowerCase();
	const limit = Number(limitValue) || 0;

	const scope = resolveMessageScope(mail, accountName, mailboxName);
	if (scope.error) return scope.error;
	const mailbox = scope.mailbox;

	try {
		const messages = mailbox.messages();
		const maxToCheck = Math.min(messages.length, 500);

		for (let i = 0; i < maxToCheck && result.length < limit; i++) {
			const message = messages[i];
			try {
				const subject = (message.subject() || '').toLowerCase();
				const sender = (message.sender() || '').toLowerCase();
				if (subject.includes(searchTerm) || sender.includes(searchTerm)) {
					result.push(messageMetadata(message, accountName, mailboxName));
				}
			} catch (e) {
				// Skip inaccessible rows.
			}
		}
	} catch (e) {
		// Unavailable mailboxes produce an empty result.
	}

	return result;
}

function specialMailboxMessages(mail, args) {
	const [mailboxType, limitValue] = args;
	const limit = Number(limitValue) || 0;
	const result = [];
	let mailboxes;

	switch (mailboxType) {
	case 'sent':
		mailboxes = mail.sentMailboxes();
		break;
	case 'trash':
		mailboxes = mail.trashMailboxes();
		break;
	case 'junk':
		mailboxes = mail.junkMailboxes();
		break;
	default:
		return result;
	}

	for (const mailbox of mailboxes) {
		let accountName = '';
		let mailboxName = mailboxType;
		try {
			accountName = mailbox.account().name();
		} catch (e) {
			try {
				accountName = mailbox.account.name();
			} catch (e) {}
		}
		try {
			mailboxName = mailbox.name();
		} catch (e) {}

		let messages;
		try {
			messages = mailbox.messages();
		} catch (e) {
			continue;
		}

		const count = Math.min(messages.length, limit);
		for (let i = 0; i < count; i++) {
			const message = messages[i];
			if (isDeleted(message)) {
				continue;
			}
			try {
				result.push(messageMetadata(message, accountName, mailboxName));
			} catch (e) {
				// Skip inaccessible rows.
			}
		}
	}

	return result;
}

function showMessages(mail, args) {
	const [accountName, mailboxName, selector, ...ids] = args;
	const scope = resolveMessageScope(mail, accountName, mailboxName);
	if (scope.error) return scope.error;
	return readMessageDetails(scope.mailbox, accountName, mailboxName, selector, ids);
}
