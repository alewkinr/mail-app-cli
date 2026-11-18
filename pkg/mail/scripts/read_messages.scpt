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

function encodeMessageRead(result) {
	return typeof result === 'string' ? result : JSON.stringify(result);
}

function readRFCMessageID(message) {
	try {
		const value = message.messageId();
		return typeof value === 'string' && value.trim() ? value : null;
	} catch (e) {
		return null;
	}
}

function messageMetadata(message, account, mailbox) {
	return {
		id: String(message.id()),
		rfcMessageID: readRFCMessageID(message),
		subject: message.subject() || '',
		sender: message.sender() || '',
		dateReceived: (message.dateReceived() || new Date()).toISOString(),
		dateSent: (message.dateSent() || new Date()).toISOString(),
		read: message.readStatus(),
		flagged: message.flaggedStatus(),
		account: account,
		mailbox: mailbox
	};
}

function messageDetail(message, account, mailbox) {
	const detail = messageMetadata(message, account, mailbox);
	detail.content = message.content() || '';
	detail.messageSize = message.messageSize();
	detail.toRecipients = message.toRecipients().map(recipient => recipient.address());
	detail.ccRecipients = message.ccRecipients().map(recipient => recipient.address());
	detail.bccRecipients = message.bccRecipients().map(recipient => recipient.address());
	return detail;
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

function normalizeRFCMessageID(id) {
	const value = id.trim();
	if (value.startsWith('<') && value.endsWith('>')) {
		return value.slice(1, -1).trim();
	}
	return value;
}

function showMessages(mail, args) {
	const [accountName, mailboxName, selector, ...ids] = args;
	const scope = resolveMessageScope(mail, accountName, mailboxName);
	if (scope.error) return scope.error;
	const mailbox = scope.mailbox;
	const byRFCID = selector === 'rfc';
	const keys = byRFCID ? ids.map(normalizeRFCMessageID) : ids;
	const matches = new Map(keys.map(key => [key, []]));

	if (byRFCID) {
		for (const message of mailbox.messages()) {
			const id = readRFCMessageID(message);
			const key = id === null ? '' : normalizeRFCMessageID(id);
			if (key && matches.has(key)) {
				matches.get(key).push({localID: String(message.id()), message: message});
			}
		}
	} else {
		const localIDs = mailbox.messages.id();
		localIDs.forEach((id, index) => {
			const key = String(id);
			if (matches.has(key)) {
				matches.get(key).push({localID: key, index: index});
			}
		});
	}

	const issues = [];
	keys.forEach((key, index) => {
		const candidates = matches.get(key);
		if (candidates.length === 0) {
			issues.push({id: ids[index], reason: 'missing'});
		} else if (candidates.length > 1) {
			issues.push({
				id: ids[index],
				reason: 'ambiguous',
				localIDs: candidates.map(candidate => candidate.localID)
			});
		}
	});
	if (issues.length > 0) {
		return {issues: issues};
	}

	// Check the current IDs before reading any bodies in case indices moved.
	const selected = keys.map(key => matches.get(key)[0]);
	const messages = selected.map(candidate => byRFCID ? candidate.message : mailbox.messages.at(candidate.index));
	messages.forEach((message, index) => {
		if (String(message.id()) !== selected[index].localID) {
			issues.push({id: ids[index], reason: 'mailbox changed during lookup'});
		}
	});
	if (issues.length > 0) {
		return {issues: issues};
	}
	const details = messages.map((message, index) => {
		const detail = messageDetail(message, accountName, mailboxName);
		const expectedID = selected[index].localID;
		// Mail.app can refresh an index-based reference during a series of reads.
		if (detail.id !== expectedID || String(message.id()) !== expectedID) {
			issues.push({id: ids[index], reason: 'mailbox changed during detail read'});
		}
		return detail;
	});
	return issues.length > 0 ? {issues: issues} : {messages: details};
}
