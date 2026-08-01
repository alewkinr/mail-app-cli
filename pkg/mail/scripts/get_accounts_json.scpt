function run(argv) {
	const mail = Application('Mail');
	const accounts = mail.accounts();
	const result = [];

	for (let i = 0; i < accounts.length; i++) {
		const acc = accounts[i];
		const emailAddresses = acc.emailAddresses();
		result.push({
			id: acc.id(),
			name: acc.name(),
			emailAddress: emailAddresses.length > 0 ? emailAddresses[0] : '',
			emailAddresses: emailAddresses,
			userName: acc.userName(),
			enabled: acc.enabled()
		});
	}

	return JSON.stringify(result);
}
