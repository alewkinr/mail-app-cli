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

function messageRecipients(message) {
	return {
		toRecipients: message.toRecipients().map(recipient => recipient.address()),
		ccRecipients: message.ccRecipients().map(recipient => recipient.address()),
		bccRecipients: message.bccRecipients().map(recipient => recipient.address())
	};
}

function messageDetail(message, account, mailbox) {
	const detail = messageMetadata(message, account, mailbox);
	detail.content = message.content() || '';
	detail.messageSize = message.messageSize();
	return Object.assign(detail, messageRecipients(message));
}

function normalizeRFCMessageID(id) {
	const value = id.trim();
	if (value.startsWith('<') && value.endsWith('>')) {
		return value.slice(1, -1).trim();
	}
	return value;
}

function readMessageDetails(mailbox, accountName, mailboxName, selector, ids, savedDraftOnly) {
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
		} else if (savedDraftOnly && message.deletedStatus()) {
			issues.push({id: ids[index], reason: 'missing'});
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
