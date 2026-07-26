function run(argv) {
	const [accountName, mailboxName, messageID, attachmentName, savePath] = argv;
	const mail = Application('Mail');
	const app = Application.currentApplication();
	app.includeStandardAdditions = true;

	try {
		const acc = mail.accounts.byName(accountName);
		const mbox = acc.mailboxes.byName(mailboxName);
		const allIds = mbox.messages.id();
		const targetIdx = allIds.findIndex(id => String(id) === messageID);
		if (targetIdx < 0) {
			return 'Error: Message not found';
		}

		const attachments = mbox.messages.at(targetIdx).mailAttachments();
		for (let a = 0; a < attachments.length; a++) {
			if (attachments[a].name() === attachmentName) {
				attachments[a].save({ in: Path(savePath) });
				return 'Success';
			}
		}

		return 'Error: Attachment not found';
	} catch (e) {
		return 'Error: ' + e;
	}
}
